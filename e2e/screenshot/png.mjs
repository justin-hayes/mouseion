// Lossless PNG optimizer for the documentation screenshot. It re-filters every
// scanline with the lowest-cost filter, recompresses at maximum deflate level,
// and drops ancillary chunks. It accepts only the 8-bit, non-interlaced RGB and
// RGBA layouts that Chromium emits.
import { deflateSync, inflateSync } from 'node:zlib';
import { crc32 } from './zip.mjs';

const SIGNATURE = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);

export function optimizePng(input) {
  if (!input.subarray(0, 8).equals(SIGNATURE)) throw new Error('screenshot is not a PNG');

  let header;
  const dataChunks = [];
  for (let pos = 8; pos < input.length; ) {
    const length = input.readUInt32BE(pos);
    const type = input.toString('latin1', pos + 4, pos + 8);
    const data = input.subarray(pos + 8, pos + 8 + length);
    pos += 12 + length;
    if (type === 'IHDR') header = data;
    else if (type === 'IDAT') dataChunks.push(data);
    else if (type === 'IEND') break;
  }
  if (!header) throw new Error('screenshot PNG has no IHDR chunk');

  const width = header.readUInt32BE(0);
  const height = header.readUInt32BE(4);
  const bitDepth = header[8];
  const colorType = header[9];
  const interlace = header[12];
  if (bitDepth !== 8 || (colorType !== 2 && colorType !== 6) || interlace !== 0) {
    throw new Error(`unsupported PNG layout (bit depth ${bitDepth}, colour type ${colorType}, interlace ${interlace})`);
  }

  const bpp = colorType === 6 ? 4 : 3;
  const stride = width * bpp;
  const raw = inflateSync(Buffer.concat(dataChunks));
  if (raw.length !== (stride + 1) * height) throw new Error('screenshot PNG has unexpected scanline data');

  const pixels = unfilter(raw, stride, height, bpp);
  const filtered = refilter(pixels, stride, height, bpp);
  return assemble(header, deflateSync(filtered, { level: 9 }));
}

function unfilter(raw, stride, height, bpp) {
  const pixels = Buffer.alloc(stride * height);
  const zeros = Buffer.alloc(stride);
  for (let y = 0; y < height; y++) {
    const filter = raw[y * (stride + 1)];
    const source = raw.subarray(y * (stride + 1) + 1, (y + 1) * (stride + 1));
    const row = pixels.subarray(y * stride, (y + 1) * stride);
    const prior = y ? pixels.subarray((y - 1) * stride, y * stride) : zeros;
    for (let x = 0; x < stride; x++) {
      const left = x >= bpp ? row[x - bpp] : 0;
      const up = prior[x];
      const upLeft = x >= bpp ? prior[x - bpp] : 0;
      row[x] = (source[x] + predict(filter, left, up, upLeft)) & 0xff;
    }
  }
  return pixels;
}

function refilter(pixels, stride, height, bpp) {
  const out = Buffer.alloc((stride + 1) * height);
  const zeros = Buffer.alloc(stride);
  for (let y = 0; y < height; y++) {
    const row = pixels.subarray(y * stride, (y + 1) * stride);
    const prior = y ? pixels.subarray((y - 1) * stride, y * stride) : zeros;
    let best = null;
    let bestScore = Infinity;
    for (let filter = 0; filter < 5; filter++) {
      const candidate = Buffer.alloc(stride);
      let score = 0;
      for (let x = 0; x < stride; x++) {
        const left = x >= bpp ? row[x - bpp] : 0;
        const upLeft = x >= bpp ? prior[x - bpp] : 0;
        const value = (row[x] - predict(filter, left, prior[x], upLeft)) & 0xff;
        candidate[x] = value;
        // Signed-magnitude cost: small residuals compress best under deflate.
        score += value < 128 ? value : 256 - value;
      }
      if (score < bestScore) {
        bestScore = score;
        best = { filter, candidate };
      }
    }
    out[y * (stride + 1)] = best.filter;
    best.candidate.copy(out, y * (stride + 1) + 1);
  }
  return out;
}

function predict(filter, left, up, upLeft) {
  switch (filter) {
    case 0:
      return 0;
    case 1:
      return left;
    case 2:
      return up;
    case 3:
      return (left + up) >> 1;
    case 4: {
      const estimate = left + up - upLeft;
      const distanceLeft = Math.abs(estimate - left);
      const distanceUp = Math.abs(estimate - up);
      const distanceUpLeft = Math.abs(estimate - upLeft);
      if (distanceLeft <= distanceUp && distanceLeft <= distanceUpLeft) return left;
      return distanceUp <= distanceUpLeft ? up : upLeft;
    }
    default:
      throw new Error(`unsupported PNG filter ${filter}`);
  }
}

function assemble(header, compressed) {
  const chunks = [SIGNATURE, chunk('IHDR', header), chunk('IDAT', compressed), chunk('IEND', Buffer.alloc(0))];
  return Buffer.concat(chunks);
}

function chunk(type, data) {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length, 0);
  const typeAndData = Buffer.concat([Buffer.from(type, 'latin1'), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(typeAndData), 0);
  return Buffer.concat([length, typeAndData, crc]);
}
