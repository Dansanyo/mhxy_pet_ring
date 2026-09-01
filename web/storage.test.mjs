import test from 'node:test'
import assert from 'node:assert/strict'

import { createRepository } from './storage.mjs'

class MemoryStorage {
  constructor() { this.values = new Map() }
  getItem(key) { return this.values.has(key) ? this.values.get(key) : null }
  setItem(key, value) { this.values.set(key, value) }
  removeItem(key) { this.values.delete(key) }
}

test('首次使用创建带未选择匿名状态的默认数据', () => {
  const repository = createRepository(new MemoryStorage(), {
    now: () => '2026-09-01T00:00:00.000Z',
    createID: () => 'generated-device-id',
  })
  const state = repository.load()
  assert.equal(state.version, 1)
  assert.equal(state.consent, null)
  assert.equal(state.deviceId, 'generated-device-id')
  assert.equal(state.current.entries.length, 0)
})

test('损坏的本地JSON不会阻断应用', () => {
  const storage = new MemoryStorage()
  storage.setItem('pet-ring:v1', '{broken')
  const repository = createRepository(storage, { createID: () => 'safe-device-id' })
  assert.equal(repository.load().deviceId, 'safe-device-id')
})

test('添加和撤销单环记录会持久化', () => {
  const storage = new MemoryStorage()
  const repository = createRepository(storage, { createID: () => 'device-id-123456' })
  repository.addEntry({ id: 'entry-1', ringNumber: 1, taskType: 'find_person', score: 1, cost: 0 })
  assert.equal(repository.load().current.entries.length, 1)
  assert.equal(repository.undoEntry().taskType, 'find_person')
  assert.equal(repository.load().current.entries.length, 0)
})

test('可以删除指定的误录环并保留其他记录', () => {
  const repository = createRepository(new MemoryStorage(), { createID: () => 'device-id-123456' })
  repository.addEntry({ id: 'entry-1', ringNumber: 1, taskType: 'find_person', score: 1, cost: 0 })
  repository.addEntry({ id: 'entry-2', ringNumber: 2, taskType: 'flower_instrument', score: 4, cost: 10 })
  repository.deleteEntry('entry-1')
  assert.deepEqual(repository.load().current.entries.map(item => [item.id, item.ringNumber]), [['entry-2', 1]])
})

test('完成周期保存历史并清空当前记录', () => {
  const storage = new MemoryStorage()
  const repository = createRepository(storage, {
    now: () => '2026-09-01T00:00:00.000Z',
    createID: () => 'device-id-123456',
  })
  repository.addEntry({ id: 'entry-1', ringNumber: 100, taskType: 'find_person', score: 1, cost: 2 })
  repository.completeCycle({ type: 'book', level: 130, value: 900 })
  const state = repository.load()
  assert.equal(state.history.length, 1)
  assert.equal(state.history[0].reward.level, 130)
  assert.equal(state.current.entries.length, 0)
})

test('匿名选择和本地物价可修改', () => {
  const repository = createRepository(new MemoryStorage(), { createID: () => 'device-id-123456' })
  repository.setConsent(true)
  repository.savePrices({ tasks: { flower_instrument: 12.5 }, rewards: { training_fruit: 80 } })
  const state = repository.load()
  assert.equal(state.consent, true)
  assert.equal(state.prices.tasks.flower_instrument, 12.5)
  assert.equal(state.prices.rewards.training_fruit, 80)
})

test('删除尚未上传的本地记录会同时取消待提交事件', () => {
  const repository = createRepository(new MemoryStorage(), { createID: () => 'device-id-123456' })
  repository.addEntry({ id: 'entry-1', ringNumber: 1, taskType: 'find_person', score: 1, cost: 0 })
  repository.queueEvent({ kind: 'task', eventId: 'entry-1', body: {} })
  repository.deleteEntry('entry-1')
  assert.equal(repository.load().pendingEvents.length, 0)
})

test('重置本期会取消本期尚未上传的任务事件并保留删除请求', () => {
  const repository = createRepository(new MemoryStorage(), { createID: () => 'device-id-123456' })
  repository.addEntry({ id: 'entry-1', ringNumber: 1, taskType: 'find_person', score: 1, cost: 0 })
  repository.queueEvent({ kind: 'task', eventId: 'entry-1', body: {} })
  repository.queueEvent({ kind: 'task_delete', eventId: 'delete-request-1', body: { eventIds: ['entry-1'] } })
  repository.resetCurrent()
  const state = repository.load()
  assert.equal(state.current.entries.length, 0)
  assert.deepEqual(state.pendingEvents.map(item => item.kind), ['task_delete'])
})

test('旧版变异选项迁移为任务要求和处理方式', () => {
  const storage = new MemoryStorage()
  const repository = createRepository(storage, { createID: () => 'device-id-123456' })
  const state = repository.load()
  state.current.entries.push({ id: 'legacy', ringNumber: 1, taskType: 'normal_pet_as_mutant', score: -15, cost: 3 })
  repository.save(state)
  const entry = repository.load().current.entries[0]
  assert.equal(entry.requestedTaskType, 'mutant_specific')
  assert.equal(entry.resolution, 'normal_pet')
})

test('旧版花和乐器记录及价格合并为花乐', () => {
  const storage = new MemoryStorage()
  const repository = createRepository(storage, { createID: () => 'device-id-123456' })
  const state = repository.load()
  state.current.entries.push({ id: 'legacy-flower', ringNumber: 1, taskType: 'flower', score: 4, cost: 8 })
  state.prices.tasks = { flower: 8, instrument: 6 }
  repository.save(state)
  const migrated = repository.load()
  assert.equal(migrated.current.entries[0].requestedTaskType, 'flower_instrument')
  assert.deepEqual(migrated.prices.tasks, { flower_instrument: 8 })
})
