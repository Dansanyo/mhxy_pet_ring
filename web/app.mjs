import { TASK_RESOLUTIONS, TASK_RULES, canChooseResolution, emptyEntryDraft, expectedRewardValue, resolutionCostKey, resolutionsForTask, rewardPriceKey, scoreResolution, simulateProjection } from './model.mjs'
import { createRepository } from './storage.mjs'

const repository = createRepository(localStorage)
const THEME_KEY = 'pet-ring:theme'
const THEMES = new Set(['light', 'system', 'dark'])
let state = repository.load()
let publicModel = { taskSamples: 0, rewardSamples: 0, taskBuckets: [], rewardBuckets: [], updatedAt: null }
let selectedTask = 'find_person'
let selectedResolution = 'fulfilled'
let toastTimer

const $ = selector => document.querySelector(selector)
const $$ = selector => [...document.querySelectorAll(selector)]
const taskRules = new Map(TASK_RULES.map(rule => [rule.id, rule]))
const taskIcons = {
  find_person: '🧭',
  equipment_60: '🗡️',
  furniture_1: '🪑',
  medicine: '🧪',
  cooking: '🍲',
  equipment_70: '⚔️',
  instrument: '🎵',
  flower: '🌸',
  equipment_80: '🛡️',
  furniture_2: '🏮',
  mutant_specific: '🐾',
}

const rewardPriceDefinitions = [
  ...['book', 'iron'].flatMap(type => [90, 100, 110, 120, 130, 140, 150].map(level => ({ key: rewardPriceKey(type, level), label: `${level}级${type === 'book' ? '书' : '铁'}` }))),
  { key: 'training_fruit', label: '修炼果' },
  { key: 'training_exp', label: '200点修炼经验' },
  { key: 'furniture_plan', label: '三级家具图' },
  { key: 'war_soul_160', label: '160级战魄' },
  { key: 'other', label: '其他奖励' },
]

initialize()

function initialize() {
  applyTheme(loadTheme())
  renderTaskButtons()
  renderPriceInputs()
  bindEvents()
  hydrateInputs()
  render()
  if (state.consent === null) $('#consent-dialog').showModal()
  loadPublicModel()
  flushPendingEvents()
}

function bindEvents() {
  $$('[data-theme-option]').forEach(button => button.addEventListener('click', () => {
    localStorage.setItem(THEME_KEY, button.dataset.themeOption)
    applyTheme(button.dataset.themeOption)
  }))
  $$('.tab').forEach(button => button.addEventListener('click', () => activateTab(button.dataset.tab)))
  $('#add-entry').addEventListener('click', addEntry)
  $('#undo-entry').addEventListener('click', () => { repository.undoEntry(); refresh('已撤销上一环') })
  $('#reset-cycle').addEventListener('click', resetCycle)
  $('#complete-cycle').addEventListener('click', () => $('#reward-dialog').showModal())
  $('#reward-type').addEventListener('change', updateRewardLevelVisibility)
  $('#save-reward').addEventListener('click', event => { event.preventDefault(); completeCycle() })
  $('#consent-share').addEventListener('click', () => { repository.setConsent(true); refresh('感谢参与匿名统计') })
  $('#consent-local').addEventListener('click', () => { repository.setConsent(false); refresh('已选择仅本地使用') })
  $('#consent-toggle').addEventListener('change', event => { repository.setConsent(event.target.checked); refresh(event.target.checked ? '已开启匿名统计' : '已关闭匿名统计') })
  $('#show-data-fields').addEventListener('click', () => $('#fields-dialog').showModal())
  $('#save-settings').addEventListener('click', saveSettings)
  $('#player-level').addEventListener('change', updateCycleSetup)
  $('#initial-ring').addEventListener('change', updateCycleSetup)
  $('#initial-score').addEventListener('change', updateCycleSetup)
  $('#initial-cost').addEventListener('change', updateCycleSetup)
  $('#required-quality').addEventListener('input', updateCalculatedScore)
  $('#actual-quality').addEventListener('input', updateCalculatedScore)
  $('#entry-resolution').addEventListener('change', event => {
    selectedResolution = event.target.value
    updateResolutionUI()
  })
  $('#entry-list').addEventListener('click', event => {
    const button = event.target.closest('[data-delete-entry]')
    if (button) { repository.deleteEntry(button.dataset.deleteEntry); refresh('已删除该条记录') }
  })
  $('#history-list').addEventListener('click', event => {
    const button = event.target.closest('[data-delete-history]')
    if (button && confirm('确定删除这期本地历史吗？')) { repository.deleteHistory(button.dataset.deleteHistory); refresh('已删除历史记录') }
  })
}

function loadTheme() {
  const saved = localStorage.getItem(THEME_KEY)
  return THEMES.has(saved) ? saved : 'system'
}

function applyTheme(theme) {
  const selected = THEMES.has(theme) ? theme : 'system'
  document.documentElement.dataset.theme = selected
  $$('[data-theme-option]').forEach(button => {
    const active = button.dataset.themeOption === selected
    button.classList.toggle('is-active', active)
    button.setAttribute('aria-pressed', String(active))
  })
}

function renderTaskButtons() {
  $('#task-grid').innerHTML = TASK_RULES.map(rule => `
    <button type="button" class="task-option${rule.id === selectedTask ? ' is-selected' : ''}" data-task="${rule.id}" aria-pressed="${rule.id === selectedTask}">
      <span class="task-icon" aria-hidden="true">${taskIcons[rule.id] || '📦'}</span>
      <span class="task-copy"><strong>${rule.name}</strong><small>${formatSigned(rule.score)} 分</small></span>
    </button>`).join('')
  $('#task-grid').addEventListener('click', event => {
    const button = event.target.closest('[data-task]')
    if (!button) return
    selectedTask = button.dataset.task
    selectedResolution = 'fulfilled'
    $$('.task-option').forEach(item => {
      const active = item.dataset.task === selectedTask
      item.classList.toggle('is-selected', active)
      item.setAttribute('aria-pressed', String(active))
    })
    renderResolutionOptions()
  })
  renderResolutionOptions()
}

function renderResolutionOptions(totals = currentTotals()) {
  const options = resolutionsForTask(selectedTask)
  if (!options.some(option => option.id === selectedResolution)) selectedResolution = 'fulfilled'
  if (!canChooseResolution(selectedResolution, totals.score)) selectedResolution = 'fulfilled'
  $('#entry-resolution').innerHTML = options.map(option =>
    `<option value="${option.id}"${option.id === selectedResolution ? ' selected' : ''}${canChooseResolution(option.id, totals.score) ? '' : ' disabled'}>${option.name}${option.id === 'skipped' && totals.score < 20 ? '（需至少20分）' : ''}</option>`,
  ).join('')
  updateResolutionUI(totals)
}

function updateResolutionUI(totals = currentTotals()) {
  if (!canChooseResolution(selectedResolution, totals.score)) {
    selectedResolution = 'fulfilled'
    return renderResolutionOptions(totals)
  }
  const rule = taskRules.get(selectedTask)
  $('#quality-fields').hidden = !rule.needsQuality || selectedResolution !== 'quality_below'
  const costKey = resolutionCostKey(selectedTask, selectedResolution)
  $('#entry-cost').value = costKey ? state.prices.tasks[costKey] ?? 0 : 0
  $('#entry-cost').disabled = selectedResolution === 'skipped'
  updateCalculatedScore()
  renderDecisionComparison(totals)
}

function renderPriceInputs() {
  const taskPrices = [
    ...TASK_RULES,
    { id: 'mutant_unspecified', name: '非指定变异召唤兽' },
    { id: 'normal_pet_as_mutant', name: '非变异召唤兽' },
  ]
  $('#task-price-grid').innerHTML = taskPrices.map(rule => `
    <label>${rule.name}<input type="number" min="0" step="0.1" inputmode="decimal" data-task-price="${rule.id}"></label>`).join('')
  $('#reward-price-grid').innerHTML = rewardPriceDefinitions.map(item => `
    <label>${item.label}<input type="number" min="0" step="0.1" inputmode="decimal" data-reward-price="${item.key}"></label>`).join('')
}

function hydrateInputs() {
  $('#player-level').value = state.playerLevel
  $('#initial-ring').value = state.current.initialRing
  $('#initial-score').value = state.current.initialScore
  $('#initial-cost').value = state.current.initialCost
  $('#consent-toggle').checked = state.consent === true
  $$('[data-task-price]').forEach(input => { input.value = state.prices.tasks[input.dataset.taskPrice] ?? 0 })
  $$('[data-reward-price]').forEach(input => { input.value = state.prices.rewards[input.dataset.rewardPrice] ?? 0 })
  const costKey = resolutionCostKey(selectedTask, selectedResolution)
  $('#entry-cost').value = costKey ? state.prices.tasks[costKey] ?? 0 : 0
}

function currentTotals() {
  const entries = state.current.entries
  return {
    ring: Math.min(100, Number(state.current.initialRing) + entries.length),
    score: Number(state.current.initialScore) + entries.reduce((sum, item) => sum + Number(item.score), 0),
    cost: Number(state.current.initialCost) + entries.reduce((sum, item) => sum + Number(item.cost), 0),
  }
}

function render() {
  state = repository.load()
  const totals = currentTotals()
  const projectionInput = {
    currentRing: totals.ring,
    currentScore: totals.score,
    currentCost: totals.cost,
    playerLevel: state.playerLevel,
    taskBuckets: publicModel.taskBuckets,
    prices: state.prices.tasks,
  }
  const projection = simulateProjection(projectionInput)
  const rewardEstimate = Number.isFinite(projection.expectedScore) ? expectedRewardValue({
    rewardBuckets: publicModel.rewardBuckets,
    playerLevel: state.playerLevel,
    expectedScore: projection.expectedScore,
    rewardPrices: state.prices.rewards,
  }) : { value: null, sampleCount: 0 }
  const profit = rewardEstimate.value === null ? null : rewardEstimate.value - projection.expectedCost

  $('#current-ring').textContent = totals.ring
  $('#progress-percent').textContent = `${totals.ring}%`
  $('#progress-bar').style.width = `${totals.ring}%`
  $('#current-score').textContent = totals.score
  $('#current-cost').textContent = formatMoney(totals.cost)
  $('#average-score').textContent = totals.ring ? (totals.score / totals.ring).toFixed(2) : '—'
  $('#expected-score').textContent = Number.isFinite(projection.expectedScore) ? projection.expectedScore : '—'
  $('#expected-cost').textContent = Number.isFinite(projection.expectedCost) ? formatMoney(projection.expectedCost) : '—'
  $('#expected-profit').textContent = profit === null ? '样本不足' : formatSignedMoney(profit)
  $('#expected-profit').style.color = profit !== null && profit < 0 ? 'var(--red)' : ''
  $('#next-ring').textContent = Math.min(100, totals.ring + 1)
  $('#complete-cycle').disabled = totals.ring < 100
  $('#add-entry').disabled = totals.ring >= 100
  $('#confidence-label').textContent = confidenceText(projection.confidence, projection.sampleCount)
  $('#score-range').textContent = projection.method === 'insufficient'
    ? '完成至少 10 环后开始预测'
    : projection.method === 'average'
      ? `按当前平均每环外推，预计 ${projection.expectedScore} 分`
      : `预计 ${projection.expectedScore} 分，较可能落在 ${projection.p10Score}–${projection.p90Score} 分`
  renderTierProbabilities(projection)
  $('#reward-value-copy').textContent = rewardEstimate.value === null
    ? '奖励概率样本不足，暂不计算期望价值。'
    : `基于 ${rewardEstimate.sampleCount} 条奖励样本，期望奖励价值约 ${formatMoney(rewardEstimate.value)}。`
  renderEntries()
  renderHistory()
  $('#consent-toggle').checked = state.consent === true
  renderResolutionOptions(totals)
}

function renderTierProbabilities(projection) {
  const tiers = [90, 100, 110, 120, 130, 140, 150]
  const ranked = tiers
    .map(tier => ({ tier, probability: projection.tierProbabilities?.[tier] }))
    .filter(item => Number.isFinite(item.probability) && item.probability > 0)
    .sort((left, right) => right.probability - left.probability || right.tier - left.tier)
    .slice(0, 3)
  $('#tier-probabilities').innerHTML = ranked.length
    ? ranked.map((item, index) => {
      const percentage = Math.round(item.probability * 100)
      return `<div class="tier-row"><span class="tier-rank">${index + 1}</span><strong>${item.tier}级</strong><div class="tier-track"><span style="width:${percentage}%"></span></div><span>${percentage}%</span></div>`
    }).join('')
    : '<p class="empty">样本充足后显示概率前三档位。</p>'
}

function renderDecisionComparison(totals) {
  const container = $('#decision-comparison')
  if (!container) return
  const rule = taskRules.get(selectedTask)
  const requiredQuality = $('#required-quality').value === '' ? undefined : Number($('#required-quality').value)
  const actualQuality = $('#actual-quality').value === '' ? undefined : Number($('#actual-quality').value)
  container.innerHTML = resolutionsForTask(selectedTask).map(option => {
    let score = option.id === 'fulfilled' ? rule.score : option.score
    let scoreKnown = Number.isFinite(score)
    if (option.id === 'quality_below' && requiredQuality !== undefined && actualQuality !== undefined) {
      try {
        score = scoreResolution(selectedTask, option.id, requiredQuality, actualQuality)
        scoreKnown = true
      } catch { scoreKnown = false }
    }
    const costKey = resolutionCostKey(selectedTask, option.id)
    const cost = costKey ? Number(state.prices.tasks[costKey] || 0) : 0
    const available = canChooseResolution(option.id, totals.score)
    const scoreCopy = scoreKnown ? `${formatSigned(score)} 分` : '填写品质后计算'
    const resultCopy = !available ? '当前积分不足 20，无法跳过' : scoreKnown ? `选择后 ${totals.score + score} 分` : '需要要求品质与实际品质'
    return `<article class="decision-option${option.id === selectedResolution ? ' is-selected' : ''}${available ? '' : ' is-disabled'}"><strong>${escapeHTML(option.name)}</strong><span>${scoreCopy} · ${formatMoney(cost)}</span><small>${resultCopy}</small></article>`
  }).join('')
}

function renderEntries() {
  const entries = [...state.current.entries].reverse()
  $('#entry-count').textContent = `${entries.length} 条`
  if (!entries.length) {
    $('#entry-list').innerHTML = '<p class="empty">还没有记录，完成一环后从上方添加。</p>'
    return
  }
  $('#entry-list').innerHTML = entries.map(entry => `
    <div class="entry-row"><strong>${entry.ringNumber} 环</strong><span>${escapeHTML(entryLabel(entry))}</span><strong>${formatSigned(entry.score)}分</strong><span class="entry-cost-value">${formatMoney(entry.cost)}</span><button class="icon-button" type="button" data-delete-entry="${entry.id}" aria-label="删除第${entry.ringNumber}环">删除</button></div>`).join('')
}

function renderHistory() {
  $('#history-count').textContent = `${state.history.length} 期`
  if (!state.history.length) {
    $('#history-list').innerHTML = '<div class="panel"><p class="empty">完成第 100 环后，记录会保存在这里。</p></div>'
    return
  }
  $('#history-list').innerHTML = state.history.map(item => {
    const profit = Number(item.reward?.value || 0) - Number(item.totalCost || 0)
    return `<article class="history-card"><div class="history-top"><div><strong>${new Date(item.completedAt).toLocaleDateString('zh-CN')} · ${item.playerLevel}级</strong><p class="muted">${rewardLabel(item.reward)}</p></div><button type="button" class="history-delete" data-delete-history="${item.id}" aria-label="删除该历史周期"><span aria-hidden="true">×</span>删除</button></div><div class="history-metrics"><div><span>最终积分</span><strong>${item.finalScore}</strong></div><div><span>总成本</span><strong>${formatMoney(item.totalCost)}</strong></div><div><span>奖励估值</span><strong>${formatMoney(item.reward?.value || 0)}</strong></div><div><span>净收益</span><strong>${formatSignedMoney(profit)}</strong></div></div></article>`
  }).join('')
}

function addEntry() {
  hideError()
  const totals = currentTotals()
  if (totals.ring >= 100) return showError('本周期已满 100 环，请先记录奖励。')
  if (!canChooseResolution(selectedResolution, totals.score)) return showError('当前积分不足 20，无法跳过本环。')
  const rule = taskRules.get(selectedTask)
  const needsQuality = rule.needsQuality && selectedResolution === 'quality_below'
  const requiredQuality = needsQuality ? Number($('#required-quality').value) : undefined
  const actualQuality = needsQuality ? Number($('#actual-quality').value) : undefined
  let score
  try { score = scoreResolution(selectedTask, selectedResolution, requiredQuality, actualQuality) } catch (error) { return showError(error.message) }
  const cost = Number($('#entry-cost').value)
  if (!Number.isFinite(cost) || cost < 0) return showError('请填写有效的本环成本。')
  const entry = { id: createID(), ringNumber: totals.ring + 1, requestedTaskType: selectedTask, resolution: selectedResolution, score, cost, requiredQuality, actualQuality, createdAt: new Date().toISOString() }
  repository.addEntry(entry)
  state = repository.load()
  if (state.consent === true) {
    repository.queueEvent({ kind: 'task', eventId: entry.id, body: { eventId: entry.id, deviceId: state.deviceId, ringNumber: entry.ringNumber, playerLevel: state.playerLevel, taskType: selectedTask, resolution: selectedResolution, ...(needsQuality ? { requiredQuality, actualQuality } : {}) } })
  }
  const emptyDraft = emptyEntryDraft()
  $('#required-quality').value = emptyDraft.requiredQuality
  $('#actual-quality').value = emptyDraft.actualQuality
  $('#entry-cost').value = emptyDraft.cost
  refresh(`已记录第 ${entry.ringNumber} 环`)
  flushPendingEvents()
}

function updateCalculatedScore() {
  const rule = taskRules.get(selectedTask)
  let value = selectedResolution === 'fulfilled' ? rule.score : TASK_RESOLUTIONS[selectedResolution].score
  if (rule.needsQuality && selectedResolution === 'quality_below') {
    if ($('#required-quality').value === '' || $('#actual-quality').value === '') {
      $('#calculated-score').textContent = '待填写'
      return
    }
    try { value = scoreResolution(selectedTask, selectedResolution, Number($('#required-quality').value), Number($('#actual-quality').value)) } catch {
      $('#calculated-score').textContent = '请检查品质'
      return
    }
  }
  $('#calculated-score').textContent = `${formatSigned(value)} 分`
}

function updateCycleSetup() {
  const initialRing = clampNumber($('#initial-ring').value, 0, 99)
  if (initialRing + state.current.entries.length > 100) return showError('已有环数与当前明细之和不能超过 100。')
  repository.setPlayerLevel(clampNumber($('#player-level').value, 1, 200))
  repository.updateCurrent({ initialRing, initialScore: Number($('#initial-score').value) || 0, initialCost: Math.max(0, Number($('#initial-cost').value) || 0) })
  refresh()
}

function resetCycle() {
  if (!confirm('确定清空当前周期吗？本地历史不会受到影响。')) return
  repository.resetCurrent()
  hydrateInputs()
  refresh('当前周期已重置')
}

function completeCycle() {
  const totals = currentTotals()
  if (totals.ring < 100) return
  const type = $('#reward-type').value
  const level = ['book', 'iron'].includes(type) ? Number($('#reward-level').value) : type === 'war_soul' ? 160 : 0
  const value = Math.max(0, Number($('#reward-value').value) || 0)
  const eventID = createID()
  if (state.consent === true) repository.queueEvent({ kind: 'reward', eventId: eventID, body: { eventId: eventID, deviceId: state.deviceId, playerLevel: state.playerLevel, finalScore: totals.score, rewardType: type, rewardLevel: level } })
  repository.completeCycle({ type, level, value })
  $('#reward-dialog').close()
  hydrateInputs()
  refresh('本周期已保存到本地历史')
  flushPendingEvents()
}

function updateRewardLevelVisibility() {
  const type = $('#reward-type').value
  $('#reward-level-wrap').hidden = !['book', 'iron'].includes(type)
}

function saveSettings() {
  const tasks = Object.fromEntries($$('[data-task-price]').map(input => [input.dataset.taskPrice, Math.max(0, Number(input.value) || 0)]))
  const rewards = Object.fromEntries($$('[data-reward-price]').map(input => [input.dataset.rewardPrice, Math.max(0, Number(input.value) || 0)]))
  repository.savePrices({ tasks, rewards })
  state = repository.load()
  const costKey = resolutionCostKey(selectedTask, selectedResolution)
  $('#entry-cost').value = costKey ? state.prices.tasks[costKey] ?? 0 : 0
  refresh('本地物价已保存')
}

async function loadPublicModel() {
  try {
    const response = await fetch('/api/v1/model', { headers: { Accept: 'application/json' } })
    if (!response.ok) throw new Error('model unavailable')
    publicModel = await response.json()
    $('#model-status').textContent = `公共样本 ${publicModel.taskSamples || 0} 环 · ${publicModel.rewardSamples || 0} 奖励`
  } catch {
    $('#model-status').textContent = '离线模式 · 使用本期平均值'
  }
  render()
}

async function flushPendingEvents() {
  state = repository.load()
  if (state.consent !== true || !navigator.onLine) return
  for (const event of [...state.pendingEvents]) {
    try {
      const endpoint = event.kind === 'reward' ? '/api/v1/events/rewards' : '/api/v1/events/tasks'
      const response = await fetch(endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(event.body) })
      if (response.ok || response.status === 409) repository.removePendingEvent(event.eventId)
      else if (response.status >= 400 && response.status < 500 && response.status !== 429) repository.removePendingEvent(event.eventId)
    } catch { break }
  }
}

function activateTab(name) {
  $$('.tab').forEach(button => button.classList.toggle('is-active', button.dataset.tab === name))
  $$('.page').forEach(page => page.classList.toggle('is-active', page.id === `page-${name}`))
  if (name === 'settings') hydrateInputs()
}

function refresh(message) {
  state = repository.load()
  render()
  if (message) showToast(message)
}

function showToast(message) {
  const toast = $('#toast')
  toast.textContent = message
  toast.hidden = false
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.hidden = true }, 2200)
}

function showError(message) { const element = $('#entry-error'); element.textContent = message; element.hidden = false }
function hideError() { $('#entry-error').hidden = true }
function formatMoney(value) { return `${Number(value || 0).toFixed(1)}万` }
function formatSigned(value) { return `${value >= 0 ? '+' : ''}${value}` }
function formatSignedMoney(value) { return `${value >= 0 ? '+' : ''}${Number(value || 0).toFixed(1)}万` }
function confidenceText(confidence, count) { return `${confidence === 'high' ? '高' : confidence === 'medium' ? '中' : '低'}可信度 · ${count || 0} 样本` }
function clampNumber(value, min, max) { return Math.min(max, Math.max(min, Math.trunc(Number(value) || 0))) }
function createID() { return globalThis.crypto?.randomUUID?.() || `event-${Date.now()}-${Math.random().toString(36).slice(2)}` }
function rewardLabel(reward = {}) { const names = { book: '制造指南书', iron: '百炼精铁', training_fruit: '修炼果', training_exp: '200点修炼经验', furniture_plan: '三级家具图', war_soul: '160级战魄', other: '其他' }; return `${names[reward.type] || '未记录'}${reward.level ? ` · ${reward.level}级` : ''}` }
function entryLabel(entry) {
  const taskType = entry.requestedTaskType || entry.taskType
  const taskName = taskRules.get(taskType)?.name || taskType
  const resolution = entry.resolution || 'fulfilled'
  if (resolution === 'fulfilled') return taskName
  return `${taskName} · ${TASK_RESOLUTIONS[resolution]?.name || resolution}`
}
function escapeHTML(value) { return String(value).replace(/[&<>'"]/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' })[character]) }
