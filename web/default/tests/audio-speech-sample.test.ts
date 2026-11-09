/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'bun:test'

import { buildAudioSpeechSample } from '../src/features/pricing/lib/audio-speech-sample'

const context = {
  baseUrl: 'https://api.example.com',
  apiKeyEnv: 'TEST_API_KEY',
  modelName: 's2.1-pro',
  endpointPath: '/v1/audio/speech',
}
const expectedBody = {
  model: 's2.1-pro',
  input: 'Hello world.',
  response_format: 'mp3',
}

describe('audio speech examples', () => {
  test('curl sends a speech request and saves binary audio', () => {
    const code = buildAudioSpeechSample('curl', context)
    expect(code).toContain('https://api.example.com/v1/audio/speech')
    expect(code).toContain('Authorization: Bearer $TEST_API_KEY')
    expect(code).toContain('--output speech.mp3')
    const body = code.match(/-d '([^']+)'/)?.[1]
    expect(JSON.parse(body ?? '')).toEqual(expectedBody)
    expect(code).not.toContain('messages')
  })

  test('Python saves response bytes after checking HTTP success', () => {
    const code = buildAudioSpeechSample('python', context)
    expect(code).toContain(
      'requests.post("https://api.example.com/v1/audio/speech"'
    )
    expect(code).toContain('os.environ["TEST_API_KEY"]')
    expect(code).toContain('response.raise_for_status()')
    expect(code).toContain('write_bytes(response.content)')
    expect(code).toContain(`json=${JSON.stringify(expectedBody)}`)
    expect(code).not.toContain('response.json()')
  })

  test('JavaScript and TypeScript execute a binary speech download', async () => {
    const code = buildAudioSpeechSample('javascript', context)
    expect(buildAudioSpeechSample('typescript', context)).toBe(code)
    const execute = new Function(
      'fetch',
      'writeFile',
      'process',
      `return (async () => {${code.replace("import { writeFile } from 'node:fs/promises'", '')}})()`
    )
    const audio = new Uint8Array([0xff, 0xfb, 0x90, 0x00])
    let requestBody: unknown
    let saved: Uint8Array | undefined
    await execute(
      async (url: string, init: RequestInit) => {
        expect(url).toBe('https://api.example.com/v1/audio/speech')
        expect(init.method).toBe('POST')
        expect(init.headers).toEqual({
          Authorization: 'Bearer example-key',
          'Content-Type': 'application/json',
        })
        requestBody = JSON.parse(String(init.body))
        return { ok: true, arrayBuffer: async () => audio.buffer }
      },
      async (path: string, data: Uint8Array) => {
        expect(path).toBe('speech.mp3')
        saved = data
      },
      { env: { TEST_API_KEY: 'example-key' } }
    )
    expect(requestBody).toEqual(expectedBody)
    expect(saved).toEqual(audio)
  })

  test('HTTP failure is not written as an audio file', async () => {
    const code = buildAudioSpeechSample('javascript', context)
    const execute = new Function(
      'fetch',
      'writeFile',
      'process',
      `return (async () => {${code.replace("import { writeFile } from 'node:fs/promises'", '')}})()`
    )
    let saved = false
    await expect(
      execute(
        async () => ({ ok: false, text: async () => 'speech rejected' }),
        async () => {
          saved = true
        },
        { env: { TEST_API_KEY: 'example-key' } }
      )
    ).rejects.toThrow('speech rejected')
    expect(saved).toBe(false)
  })
})
