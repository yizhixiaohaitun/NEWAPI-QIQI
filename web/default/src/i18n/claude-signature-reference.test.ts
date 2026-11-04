// @ts-expect-error Bun provides its test runner at runtime.
import { test } from 'bun:test'
import assert from 'node:assert/strict'
import i18n from './config'
import './claude-signature-reference'
import './token-request-compatibility'

test('signature reference and token capability are translated', async () => {
  await i18n.changeLanguage('en')
  assert.equal(i18n.t('历史思考签名兼容'), 'Historical thinking signature compatibility')
  assert.match(i18n.t('已清理 {{referenceCount}} 个历史思考块，并在同渠道重试 1 次。', { referenceCount: 3 }), /3 historical thinking blocks/)
  await i18n.changeLanguage('zhCN')
  assert.match(i18n.t('Only on the exact historical thinking signature error before output, removes thinking blocks from completed turns and retries once on the same channel; thinking stays enabled and success is not guaranteed.'), /不保证成功/)
})
