// node --test src/lib/voiceReconnect.test.ts (npm test). Fora do tsconfig do
// app: roda no Node com a remoção de tipos dele, sem bundler.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { REJOIN_WINDOW_MS, isFinalRejoinError, rejoinDelay, shouldRejoin } from './voiceReconnect.ts'

test('reconecta só quando a conexão caiu', () => {
  // livekit-client desistindo de reconectar: sem motivo.
  assert.equal(shouldRejoin(undefined), true)
  for (const reason of [0, 3, 6, 7, 8, 9, 14, 15]) assert.equal(shouldRejoin(reason), true, `motivo ${reason}`)
  // Saiu, trocou de canal ou foi movido; mesma conta em outro aparelho;
  // expulso; sala apagada ou fechada.
  for (const reason of [1, 2, 4, 5, 10, 11, 12, 13, 16, 99]) {
    assert.equal(shouldRejoin(reason), false, `motivo ${reason}`)
  }
})

test('espera crescente e desiste depois da janela', () => {
  assert.deepEqual(
    [0, 1, 2, 3, 4, 9].map((attempt) => rejoinDelay(attempt, 0)),
    [1_000, 2_000, 5_000, 10_000, 10_000, 10_000],
  )
  // A última espera não passa do fim da janela.
  assert.equal(rejoinDelay(5, REJOIN_WINDOW_MS - 3_000), 3_000)
  assert.equal(rejoinDelay(0, REJOIN_WINDOW_MS), undefined)
  assert.equal(rejoinDelay(2, REJOIN_WINDOW_MS + 1), undefined)
})

test('só sem permissão ou canal apagado para de tentar', () => {
  assert.equal(isFinalRejoinError(403), true)
  assert.equal(isFinalRejoinError(404), true)
  for (const status of [undefined, 401, 429, 500, 502, 503]) assert.equal(isFinalRejoinError(status), false)
})
