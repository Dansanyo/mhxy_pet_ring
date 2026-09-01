import test from 'node:test'
import assert from 'node:assert/strict'

import {
  TASK_RULES,
  emptyEntryDraft,
  expectedRewardValue,
  fallbackProjection,
  rewardPriceKey,
  resolutionsForTask,
  rewardThreshold,
  rewardTier,
  scoreResolution,
  scoreTask,
  simulateProjection,
} from './model.mjs'

test('完成一环后清空品质并将本环成本归零', () => {
  assert.deepEqual(emptyEntryDraft(), {
    requiredQuality: '',
    actualQuality: '',
    cost: 0,
  })
})

test('任务规则只包含系统要求，不混入玩家处理方式', () => {
  assert.equal(TASK_RULES.length, 11)
  assert.equal(TASK_RULES.find(item => item.id === 'mutant_specific').score, 10)
  assert.equal(TASK_RULES.some(item => item.id === 'normal_pet_as_mutant'), false)
})

test('指定变异任务提供四种处理方式', () => {
  assert.deepEqual(resolutionsForTask('mutant_specific').map(item => item.id), [
    'fulfilled', 'mutant_unspecified', 'normal_pet', 'skipped',
  ])
  assert.equal(scoreResolution('mutant_specific', 'fulfilled'), 10)
  assert.equal(scoreResolution('mutant_specific', 'mutant_unspecified'), 5)
  assert.equal(scoreResolution('mutant_specific', 'normal_pet'), -15)
  assert.equal(scoreResolution('mutant_specific', 'skipped'), -20)
})

test('所有任务都可以跳过且不会把替代交付用于其他任务', () => {
  assert.equal(scoreResolution('flower', 'skipped'), -20)
  assert.throws(() => scoreResolution('flower', 'normal_pet'), /只适用于指定变异任务/)
})

test('奖励期望值使用公共概率和本地价格', () => {
  const result = expectedRewardValue({
    rewardBuckets: [
      { levelBand: 175, scoreBucket: 200, rewardType: 'book', rewardLevel: 130, count: 3 },
      { levelBand: 175, scoreBucket: 200, rewardType: 'training_fruit', rewardLevel: 0, count: 1 },
    ],
    playerLevel: 175,
    expectedScore: 202,
    rewardPrices: { [rewardPriceKey('book', 130)]: 1000, training_fruit: 80 },
  })
  assert.deepEqual(result, { value: 770, sampleCount: 4 })
})

test('没有奖励样本时不输出伪精确价值', () => {
  assert.deepEqual(expectedRewardValue({ rewardBuckets: [] }), { value: null, sampleCount: 0 })
})

test('三级药品质不足按差值的一半向下取整扣分', () => {
  assert.equal(scoreTask('medicine', 63, 58), 0)
  assert.equal(scoreTask('medicine', 70, 60), -3)
})

test('非品质任务返回基础积分', () => {
  assert.equal(scoreTask('equipment_80'), 5)
})

test('品质任务缺少品质时报错', () => {
  assert.throws(() => scoreTask('cooking'), /品质/)
})

test('奖励门槛公式匹配175级表格', () => {
  assert.equal(rewardThreshold(175, 150), 222)
  assert.equal(rewardTier(175, 202), 130)
  assert.equal(rewardTier(175, 161), 0)
})

test('无公共模型时按当前平均值外推', () => {
  assert.deepEqual(fallbackProjection({
    currentRing: 75,
    currentScore: 137,
    currentCost: 712.8,
    playerLevel: 175,
  }), {
    expectedScore: 183,
    p10Score: 183,
    p50Score: 183,
    p90Score: 183,
    expectedCost: 950.4,
    expectedTier: 110,
    tierProbabilities: null,
    confidence: 'low',
    sampleCount: 0,
  })
})

test('固定任务分布可以预测剩余积分和本地价格成本', () => {
  const result = simulateProjection({
    currentRing: 98,
    currentScore: 180,
    currentCost: 900,
    playerLevel: 175,
    taskBuckets: [{ bucket: 10, taskType: 'find_person', score: 1, count: 20 }],
    prices: { find_person: 2 },
    runs: 20,
    random: () => 0,
  })
  assert.equal(result.expectedScore, 182)
  assert.equal(result.expectedCost, 904)
  assert.equal(result.tierProbabilities[110], 1)
  assert.equal(result.tierProbabilities[120], 0)
})
