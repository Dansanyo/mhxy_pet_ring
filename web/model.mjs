export const TASK_RULES = Object.freeze([
  { id: 'find_person', name: '找人', score: 1 },
  { id: 'equipment_60', name: '60级装备', score: 2 },
  { id: 'furniture_1', name: '1级家具', score: 2 },
  { id: 'medicine', name: '三级药', score: 2, needsQuality: true },
  { id: 'cooking', name: '烹饪', score: 2, needsQuality: true },
  { id: 'equipment_70', name: '70级装备', score: 3 },
  { id: 'instrument', name: '乐器', score: 4 },
  { id: 'flower', name: '花', score: 4 },
  { id: 'equipment_80', name: '80级装备', score: 5 },
  { id: 'furniture_2', name: '2级家具', score: 5 },
  { id: 'mutant_specific', name: '指定变异', score: 10 },
])

export const TASK_RESOLUTIONS = Object.freeze({
  fulfilled: { id: 'fulfilled', name: '按要求完成' },
  quality_below: { id: 'quality_below', name: '品质不达标' },
  mutant_unspecified: { id: 'mutant_unspecified', name: '上交非指定变异', score: 5, costKey: 'mutant_unspecified' },
  normal_pet: { id: 'normal_pet', name: '上交非变异', score: -15, costKey: 'normal_pet_as_mutant' },
  skipped: { id: 'skipped', name: '跳过本环', score: -20, costKey: null },
})

export const MIN_SIMULATION_SAMPLES = 200

const taskByID = new Map(TASK_RULES.map(rule => [rule.id, rule]))

export function scoreTask(taskType, requiredQuality, actualQuality) {
  const rule = taskByID.get(taskType)
  if (!rule) throw new Error('未知任务类型')
  if (!rule.needsQuality) return rule.score
  if (!Number.isFinite(requiredQuality) || !Number.isFinite(actualQuality)) {
    throw new Error('需要填写要求品质和实际品质')
  }
  if (requiredQuality < 0 || actualQuality < 0) throw new Error('品质不能为负数')
  if (actualQuality >= requiredQuality) return rule.score
  return rule.score - Math.floor((requiredQuality - actualQuality) / 2)
}

export function resolutionsForTask(taskType) {
  const rule = taskByID.get(taskType)
  const options = rule?.needsQuality
    ? [{ ...TASK_RESOLUTIONS.fulfilled, name: '品质达标' }, TASK_RESOLUTIONS.quality_below]
    : [TASK_RESOLUTIONS.fulfilled]
  if (taskType === 'mutant_specific') {
    options.push(TASK_RESOLUTIONS.mutant_unspecified, TASK_RESOLUTIONS.normal_pet)
  }
  options.push(TASK_RESOLUTIONS.skipped)
  return options
}

export function scoreResolution(taskType, resolution = 'fulfilled', requiredQuality, actualQuality) {
  const option = TASK_RESOLUTIONS[resolution]
  if (!option) throw new Error('未知处理方式')
  if (resolution === 'fulfilled') {
    const rule = taskByID.get(taskType)
    if (rule?.needsQuality && requiredQuality === undefined && actualQuality === undefined) return rule.score
    return scoreTask(taskType, requiredQuality, actualQuality)
  }
  if (resolution === 'quality_below') {
    const rule = taskByID.get(taskType)
    if (!rule?.needsQuality) throw new Error('该处理方式只适用于品质任务')
    if (!Number.isFinite(requiredQuality) || !Number.isFinite(actualQuality)) throw new Error('需要填写要求品质和实际品质')
    if (actualQuality >= requiredQuality) throw new Error('选择品质不达标时，实际品质必须低于要求品质')
    return scoreTask(taskType, requiredQuality, actualQuality)
  }
  if (['mutant_unspecified', 'normal_pet'].includes(resolution) && taskType !== 'mutant_specific') {
    throw new Error('该处理方式只适用于指定变异任务')
  }
  return option.score
}

export function resolutionCostKey(taskType, resolution = 'fulfilled') {
  if (resolution === 'fulfilled' || resolution === 'quality_below') return taskType
  return TASK_RESOLUTIONS[resolution]?.costKey ?? null
}

export function canChooseResolution(resolution, currentScore) {
  return resolution !== 'skipped' || Number(currentScore) >= 20
}

export function rewardThreshold(playerLevel, rewardLevel) {
  if (!Number.isFinite(playerLevel) || playerLevel <= 0) return 0
  if (rewardLevel < 90 || rewardLevel > 150 || rewardLevel % 10 !== 0) return 0
  return Math.ceil(220 - playerLevel / 3) + rewardLevel - 90
}

export function rewardTier(playerLevel, finalScore) {
  for (let tier = 150; tier >= 90; tier -= 10) {
    if (finalScore >= rewardThreshold(playerLevel, tier)) return tier
  }
  return 0
}

export function fallbackProjection({ currentRing, currentScore, currentCost, playerLevel }) {
  const ring = clamp(Math.trunc(currentRing || 0), 0, 100)
  const expectedScore = ring ? Math.round((currentScore / ring) * 100) : 0
  const expectedCost = ring ? round((currentCost / ring) * 100, 1) : 0
  const expectedTier = rewardTier(playerLevel, expectedScore)
  const tierProbabilities = Object.fromEntries(
    [90, 100, 110, 120, 130, 140, 150].map(tier => [tier, tier === expectedTier ? 1 : 0]),
  )
  return {
    expectedScore,
    p10Score: expectedScore,
    p50Score: expectedScore,
    p90Score: expectedScore,
    expectedCost,
    expectedTier,
    tierProbabilities,
    confidence: 'low',
    sampleCount: 0,
    method: 'average',
  }
}

export function simulateProjection(input) {
  const taskBuckets = Array.isArray(input.taskBuckets) ? input.taskBuckets : []
  const sampleCount = taskBuckets.reduce((sum, row) => sum + positiveCount(row.count), 0)
  if (sampleCount < MIN_SIMULATION_SAMPLES) {
    return { ...fallbackProjection(input), sampleCount }
  }

  const grouped = new Map()
  for (const row of taskBuckets) {
    const bucket = Number(row.bucket)
    const rows = grouped.get(bucket) || []
    rows.push({
      taskType: row.taskType,
      score: Number(row.score) || 0,
      count: positiveCount(row.count),
    })
    grouped.set(bucket, rows)
  }
  const globalRows = taskBuckets.map(row => ({
    taskType: row.taskType,
    score: Number(row.score) || 0,
    count: positiveCount(row.count),
  }))
  const runs = Math.max(1, Math.trunc(input.runs || 3000))
  const random = input.random || Math.random
  const scores = []
  let costTotal = 0
  const tierHits = Object.fromEntries([90, 100, 110, 120, 130, 140, 150].map(tier => [tier, 0]))
  const currentRing = clamp(Math.trunc(input.currentRing || 0), 0, 100)

  for (let run = 0; run < runs; run += 1) {
    let score = Number(input.currentScore) || 0
    let cost = Number(input.currentCost) || 0
    for (let ring = currentRing + 1; ring <= 100; ring += 1) {
      const bucket = Math.floor((ring - 1) / 10) + 1
      const outcome = chooseWeighted(grouped.get(bucket) || globalRows, random)
      score += outcome.score
      cost += Number(input.prices?.[outcome.taskType]) || 0
    }
    scores.push(score)
    costTotal += cost
    const tier = rewardTier(input.playerLevel, score)
    if (tierHits[tier] !== undefined) tierHits[tier] += 1
  }

  scores.sort((a, b) => a - b)
  const expectedScore = Math.round(scores.reduce((sum, value) => sum + value, 0) / runs)
  const tierProbabilities = Object.fromEntries(
    Object.entries(tierHits).map(([tier, count]) => [tier, count / runs]),
  )
  return {
    expectedScore,
    p10Score: percentile(scores, 0.1),
    p50Score: percentile(scores, 0.5),
    p90Score: percentile(scores, 0.9),
    expectedCost: round(costTotal / runs, 1),
    expectedTier: rewardTier(input.playerLevel, expectedScore),
    tierProbabilities,
    confidence: sampleCount >= 1000 ? 'high' : sampleCount >= 200 ? 'medium' : 'low',
    sampleCount,
    method: 'simulation',
  }
}

export function expectedRewardValue({ rewardBuckets, playerLevel, expectedScore, rewardPrices }) {
  const rows = Array.isArray(rewardBuckets) ? rewardBuckets : []
  if (!rows.length) return { value: null, sampleCount: 0 }
  const levelBand = nearestLevelBand(playerLevel)
  const scoreBucket = Math.floor(expectedScore / 10) * 10
  let candidates = rows.filter(row => row.levelBand === levelBand && row.scoreBucket === scoreBucket)
  if (!candidates.length) candidates = rows.filter(row => row.levelBand === levelBand)
  if (!candidates.length) candidates = rows
  const sampleCount = candidates.reduce((sum, row) => sum + positiveCount(row.count), 0)
  if (!sampleCount) return { value: null, sampleCount: 0 }
  const value = candidates.reduce((sum, row) => {
    const key = rewardPriceKey(row.rewardType, row.rewardLevel)
    return sum + positiveCount(row.count) * (Number(rewardPrices?.[key]) || 0)
  }, 0) / sampleCount
  return { value: round(value, 1), sampleCount }
}

export function rewardPriceKey(type, level = 0) {
  return level ? `${type}_${level}` : type
}

export function emptyEntryDraft() {
  return { requiredQuality: '', actualQuality: '', cost: 0 }
}

function chooseWeighted(rows, random) {
  const total = rows.reduce((sum, row) => sum + positiveCount(row.count), 0)
  if (!total) return { taskType: 'find_person', score: 1, count: 1 }
  let target = Math.min(0.999999999, Math.max(0, random())) * total
  for (const row of rows) {
    target -= positiveCount(row.count)
    if (target < 0) return row
  }
  return rows[rows.length - 1]
}

function percentile(sorted, fraction) {
  return sorted[Math.round(fraction * (sorted.length - 1))] ?? 0
}

function positiveCount(value) {
  return Math.max(0, Math.trunc(Number(value) || 0))
}

function nearestLevelBand(level) {
  const bands = [69, 89, 109, 129, 138, 155, 159, 170, 175]
  return bands.reduce((best, band) => Math.abs(level - band) < Math.abs(level - best) ? band : best)
}

function clamp(value, minimum, maximum) {
  return Math.min(maximum, Math.max(minimum, value))
}

function round(value, digits) {
  const factor = 10 ** digits
  return Math.round((value + Number.EPSILON) * factor) / factor
}
