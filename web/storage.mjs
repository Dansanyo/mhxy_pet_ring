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
      return mutate(state => { state.current = emptyCycle(now) })
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
    history: Array.isArray(state.history) ? state.history.slice(0, MAX_HISTORY) : [],
    prices: {
      tasks: state.prices?.tasks || {},
      rewards: state.prices?.rewards || {},
    },
    pendingEvents: Array.isArray(state.pendingEvents) ? state.pendingEvents : [],
  }
}

function defaultID() {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID()
  return `local-${Date.now()}-${Math.random().toString(36).slice(2)}`
}
