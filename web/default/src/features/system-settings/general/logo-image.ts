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
export const MAX_LOGO_IMAGE_BYTES = 256 * 1024
export const MAX_LOGO_DIMENSION = 4096
export const MAX_LOGO_PIXELS = MAX_LOGO_DIMENSION * MAX_LOGO_DIMENSION

const allowedMimeTypes = new Set([
  'image/png',
  'image/jpeg',
  'image/webp',
  'image/gif',
])

function detectedMimeType(bytes: Uint8Array): string | null {
  if (
    bytes.length >= 8 &&
    bytes[0] === 0x89 &&
    bytes[1] === 0x50 &&
    bytes[2] === 0x4e &&
    bytes[3] === 0x47 &&
    bytes[4] === 0x0d &&
    bytes[5] === 0x0a &&
    bytes[6] === 0x1a &&
    bytes[7] === 0x0a
  ) {
    return 'image/png'
  }
  if (
    bytes.length >= 3 &&
    bytes[0] === 0xff &&
    bytes[1] === 0xd8 &&
    bytes[2] === 0xff
  ) {
    return 'image/jpeg'
  }
  if (
    bytes.length >= 12 &&
    String.fromCharCode(...bytes.slice(0, 4)) === 'RIFF' &&
    String.fromCharCode(...bytes.slice(8, 12)) === 'WEBP'
  ) {
    return 'image/webp'
  }
  if (bytes.length >= 6) {
    const signature = String.fromCharCode(...bytes.slice(0, 6))
    if (signature === 'GIF87a' || signature === 'GIF89a') {
      return 'image/gif'
    }
  }
  return null
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  const chunkSize = 0x8000
  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize))
  }
  return btoa(binary)
}

export function logoBytesToDataUrl(bytes: Uint8Array): string {
  if (bytes.byteLength === 0) throw new Error('The image file is empty')
  if (bytes.byteLength > MAX_LOGO_IMAGE_BYTES) {
    throw new Error('The logo image must be 256 KiB or smaller')
  }
  const mimeType = detectedMimeType(bytes)
  if (!mimeType || !allowedMimeTypes.has(mimeType)) {
    throw new Error('Choose a PNG, JPEG, WebP, or GIF image')
  }
  return `data:${mimeType};base64,${bytesToBase64(bytes)}`
}

export async function ensureLogoImageDecodes(
  value: string,
  createImage: () => Pick<
    HTMLImageElement,
    'src' | 'decode' | 'naturalWidth' | 'naturalHeight'
  > = () => new Image()
): Promise<void> {
  const image = createImage()
  image.src = value
  await image.decode()
  if (image.naturalWidth < 1 || image.naturalHeight < 1) {
    throw new Error('Choose a valid logo image')
  }
  if (
    image.naturalWidth > MAX_LOGO_DIMENSION ||
    image.naturalHeight > MAX_LOGO_DIMENSION ||
    image.naturalWidth * image.naturalHeight > MAX_LOGO_PIXELS
  ) {
    throw new Error('The logo dimensions must not exceed 4096 x 4096 pixels')
  }
}

export async function logoFileToDataUrl(file: File): Promise<string> {
  if (file.size === 0) throw new Error('The image file is empty')
  if (file.size > MAX_LOGO_IMAGE_BYTES) {
    throw new Error('The logo image must be 256 KiB or smaller')
  }
  const value = logoBytesToDataUrl(new Uint8Array(await file.arrayBuffer()))
  await ensureLogoImageDecodes(value)
  return value
}

export function isSupportedLogoValue(value: string): boolean {
  if (value === '') return true
  if (value.startsWith('data:')) {
    const match =
      /^data:(image\/(?:png|jpeg|webp|gif));base64,([A-Za-z0-9+/]+={0,2})$/.exec(
        value
      )
    if (!match) return false
    try {
      const binary = atob(match[2])
      const bytes = Uint8Array.from(binary, (character) =>
        character.charCodeAt(0)
      )
      return (
        bytes.byteLength <= MAX_LOGO_IMAGE_BYTES &&
        detectedMimeType(bytes) === match[1]
      )
    } catch {
      return false
    }
  }
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}
