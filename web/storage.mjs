const STORAGE_KEY = 'pet-ring:v1'
const MAX_HISTORY = 100

export function createRepository(storage, options = {}) {
  const now = options.now || (() => new Date().toISOString())
  const createID = options.createID || defaultID

  function defaultState() {
    return {
      version: 1,
      consent: null,
      deviceId: createID(),
      playerLevel: 175,
      current: emptyCycle(now),
      history: [],
      prices: { tasks: {}, rewards: {} },
      pendingEvents: [],
    }
  }

  function load() {
    const raw = storage.getItem(STORAGE_KEY)
    if (raw) {
      try {
        const state = JSON.parse(raw)
        if (isValidState(state)) return normalizeState(state)
      } catch { /* fall through to a safe default */ }
    }
    const state = defaultState()
    save(state)
    return state
  }

  function save(state) {
    storage.setItem(STORAGE_KEY, JSON.stringify(state))
    return state
  }

  function mutate(change) {
    const state = load()
    change(state)
    return save(state)
  }

  return {
    load,
    save,
    addEntry(entry) {
      return mutate(state => { state.current.entries.push({ ...entry }) })
    },
    undoEntry() {
      let removed = null
      mutate(state => {
        removed = state.current.entries.pop() || null
        if (removed) state.pendingEvents = state.pendingEvents.filter(item => item.eventId !== removed.id)
      })
      return removed
    },
    deleteEntry(id) {
      return mutate(state => {
        state.current.entries = state.current.entries
          .filter(item => item.id !== id)
          .map((item, index) => ({ ...item, ringNumber: state.current.initialRing + index + 1 }))
        state.pendingEvents = state.pendingEvents.filter(item => item.eventId !== id)
      })
    },
    updateCurrent(patch) {
      return mutate(state => { state.current = { ...state.current, ...patch } })
    },
    resetCurrent() {
      return mutate(state => {
        const eventIDs = new Set(state.current.entries.map(item => item.id))
        state.pendingEvents = state.pendingEvents.filter(item =>
          item.kind !== 'task' || !eventIDs.has(item.eventId))
        state.current = emptyCycle(now)
      })
    },
    completeCycle(reward) {
      return mutate(state => {
        const score = state.current.initialScore + state.current.entries.reduce((sum, item) => sum + Number(item.score || 0), 0)
        const cost = state.current.initialCost + state.current.entries.reduce((sum, item) => sum + Number(item.cost || 0), 0)
        state.history.unshift({
          id: createID(),
          completedAt: now(),
          playerLevel: state.playerLevel,
          finalScore: score,
          totalCost: cost,
          reward: { ...reward },
          entries: state.current.entries.map(item => ({ ...item })),
        })
        state.history = state.history.slice(0, MAX_HISTORY)
        state.current = emptyCycle(now)
      })
    },
    deleteHistory(id) {
      return mutate(state => { state.history = state.history.filter(item => item.id !== id) })
    },
    setConsent(value) {
      return mutate(state => { state.consent = Boolean(value) })
    },
    setPlayerLevel(value) {
      return mutate(state => { state.playerLevel = Math.min(200, Math.max(1, Math.trunc(value))) })
    },
    savePrices(prices) {
      return mutate(state => {
        state.prices = {
          tasks: { ...state.prices.tasks, ...(prices.tasks || {}) },
          rewards: { ...state.prices.rewards, ...(prices.rewards || {}) },
        }
      })
    },
    queueEvent(event) {
      return mutate(state => {
        if (!state.pendingEvents.some(item => item.eventId === event.eventId)) state.pendingEvents.push({ ...event })
      })
    },
    removePendingEvent(eventId) {
      return mutate(state => { state.pendingEvents = state.pendingEvents.filter(item => item.eventId !== eventId) })
    },
  }
}

function emptyCycle(now) {
  return {
    startedAt: now(),
    initialRing: 0,
    initialScore: 0,
    initialCost: 0,
    entries: [],
  }
}

function isValidState(state) {
  return state && state.version === 1 && typeof state.deviceId === 'string' && state.current && Array.isArray(state.current.entries)
}

function normalizeState(state) {
  return {
    ...state,
    consent: state.consent === null ? null : Boolean(state.consent),
    current: {
      ...state.current,
      entries: state.current.entries.map(normalizeEntry),
    },
    history: Array.isArray(state.history) ? state.history.slice(0, MAX_HISTORY).map(item => ({
      ...item,
      entries: Array.isArray(item.entries) ? item.entries.map(normalizeEntry) : [],
    })) : [],
    prices: {
      tasks: normalizeTaskPrices(state.prices?.tasks || {}),
      rewards: state.prices?.rewards || {},
    },
    pendingEvents: Array.isArray(state.pendingEvents) ? state.pendingEvents.map(normalizePendingEvent) : [],
  }
}

function normalizeEntry(entry) {
  const taskType = normalizeTaskType(entry.requestedTaskType || entry.taskType)
  if (entry.requestedTaskType) {
    return { ...entry, requestedTaskType: taskType, resolution: entry.resolution || 'fulfilled' }
  }
  if (entry.taskType === 'mutant_unspecified') {
    return { ...entry, requestedTaskType: 'mutant_specific', resolution: 'mutant_unspecified' }
  }
  if (entry.taskType === 'normal_pet_as_mutant') {
    return { ...entry, requestedTaskType: 'mutant_specific', resolution: 'normal_pet' }
  }
  return { ...entry, requestedTaskType: taskType, resolution: entry.resolution || 'fulfilled' }
}

function normalizePendingEvent(event) {
  if (event.kind !== 'task' || !event.body) return event
  const entry = normalizeEntry({ taskType: event.body.taskType, resolution: event.body.resolution })
  return {
    ...event,
    body: { ...event.body, taskType: entry.requestedTaskType, resolution: entry.resolution },
  }
}

function normalizeTaskType(taskType) {
  return ['flower', 'instrument'].includes(taskType) ? 'flower_instrument' : taskType
}

function normalizeTaskPrices(prices) {
  const result = { ...prices }
  if (result.flower_instrument === undefined) {
    result.flower_instrument = result.flower ?? result.instrument ?? 0
  }
  delete result.flower
  delete result.instrument
  return result
}

function defaultID() {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID()
  return `local-${Date.now()}-${Math.random().toString(36).slice(2)}`
}
