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
type AudioSpeechSampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
  endpointPath: string
}

export function buildAudioSpeechSample(
  lang: 'curl' | 'python' | 'typescript' | 'javascript',
  ctx: AudioSpeechSampleContext
): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const body = {
    model: ctx.modelName,
    input: 'Hello world.',
    response_format: 'mp3',
  }
  if (lang === 'curl') {
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${JSON.stringify(body)}' \\`,
      `  --output speech.mp3`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import os',
      'from pathlib import Path',
      'import requests',
      '',
      `response = requests.post("${url}",`,
      `    headers={"Authorization": "Bearer " + os.environ["${ctx.apiKeyEnv}"]},`,
      `    json=${JSON.stringify(body)},`,
      '    timeout=90,',
      ')',
      'response.raise_for_status()',
      'Path("speech.mp3").write_bytes(response.content)',
    ].join('\n')
  }
  return [
    `import { writeFile } from 'node:fs/promises'`,
    '',
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify(${JSON.stringify(body)}),`,
    `})`,
    `if (!response.ok) throw new Error(await response.text())`,
    `await writeFile('speech.mp3', new Uint8Array(await response.arrayBuffer()))`,
  ].join('\n')
}
