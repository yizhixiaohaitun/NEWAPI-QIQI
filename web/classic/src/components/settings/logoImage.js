/*
Copyright (C) 2025 QuantumNous

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
export const MAX_LOGO_IMAGE_BYTES = 256 * 1024;
export const MAX_LOGO_DIMENSION = 4096;
export const MAX_LOGO_PIXELS = MAX_LOGO_DIMENSION * MAX_LOGO_DIMENSION;

const detectMimeType = (bytes) => {
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
  )
    return 'image/png';
  if (
    bytes.length >= 3 &&
    bytes[0] === 0xff &&
    bytes[1] === 0xd8 &&
    bytes[2] === 0xff
  )
    return 'image/jpeg';
  const firstFour = String.fromCharCode(...bytes.slice(0, 4));
  const webp = String.fromCharCode(...bytes.slice(8, 12));
  if (bytes.length >= 12 && firstFour === 'RIFF' && webp === 'WEBP')
    return 'image/webp';
  const gif = String.fromCharCode(...bytes.slice(0, 6));
  if (gif === 'GIF87a' || gif === 'GIF89a') return 'image/gif';
  return null;
};

const toBase64 = (bytes) => {
  let binary = '';
  for (let index = 0; index < bytes.length; index += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(index, index + 0x8000));
  }
  return btoa(binary);
};

const ensureImageDecodes = async (value) => {
  const image = new Image();
  image.src = value;
  await image.decode();
  if (image.naturalWidth < 1 || image.naturalHeight < 1) {
    throw new Error('请选择有效的徽标图片');
  }
  if (
    image.naturalWidth > MAX_LOGO_DIMENSION ||
    image.naturalHeight > MAX_LOGO_DIMENSION ||
    image.naturalWidth * image.naturalHeight > MAX_LOGO_PIXELS
  ) {
    throw new Error('徽标尺寸不能超过 4096 x 4096 像素');
  }
};

export const logoFileToDataUrl = async (file) => {
  if (file.size === 0) throw new Error('图片文件不能为空');
  if (file.size > MAX_LOGO_IMAGE_BYTES)
    throw new Error('徽标图片不能超过 256 KiB');
  const bytes = new Uint8Array(await file.arrayBuffer());
  if (bytes.length === 0) throw new Error('图片文件不能为空');
  if (bytes.length > MAX_LOGO_IMAGE_BYTES)
    throw new Error('徽标图片不能超过 256 KiB');
  const mimeType = detectMimeType(bytes);
  if (!mimeType) throw new Error('请选择 PNG、JPEG、WebP 或 GIF 图片');
  const value = `data:${mimeType};base64,${toBase64(bytes)}`;
  await ensureImageDecodes(value);
  return value;
};
