import crypto from 'crypto';
import type {NextApiRequest, NextApiResponse} from 'next';

export const config = {
  api: {bodyParser: false},
};

type RevalidationPayload = {
  paths: string[];
  event_id: string;
  timestamp: number;
};

const replayWindowMs = 10 * 60 * 1000;
const seenEvents = new Map<string, number>();

export default async function handler(request: NextApiRequest, response: NextApiResponse) {
  if (request.method !== 'POST') {
    response.setHeader('Allow', 'POST');
    return response.status(405).json({revalidated: false});
  }
  const secret = process.env.SIXURL_REVALIDATION_SECRET ?? '';
  if (secret.length < 32) {
    return response.status(503).json({revalidated: false});
  }
  let body: Buffer;
  try {
    body = await readBody(request, 32 << 10);
  } catch {
    return response.status(413).json({revalidated: false});
  }
  const signature = parseSignature(request.headers['x-6url-signature']);
  if (!signature || Math.abs(Date.now() / 1000 - signature.timestamp) > 300) {
    return response.status(401).json({revalidated: false});
  }
  const expected = crypto.createHmac('sha256', secret).update(`${signature.timestamp}.`).update(body).digest('hex');
  if (!safeEqual(expected, signature.digest)) {
    return response.status(401).json({revalidated: false});
  }
  let payload: RevalidationPayload;
  try {
    payload = JSON.parse(body.toString('utf8')) as RevalidationPayload;
  } catch {
    return response.status(400).json({revalidated: false});
  }
  if (payload.timestamp !== signature.timestamp || !validPayload(payload)) {
    return response.status(400).json({revalidated: false});
  }
  const now = Date.now();
  seenEvents.forEach((expiresAt, eventID) => {
    if (expiresAt <= now) seenEvents.delete(eventID);
  });
  if (seenEvents.has(payload.event_id)) {
    return response.status(200).json({revalidated: true, replayed: true});
  }
  try {
    for (const path of payload.paths) {
      await response.revalidate(path);
    }
  } catch {
    return response.status(502).json({revalidated: false});
  }
  seenEvents.set(payload.event_id, now + replayWindowMs);
  return response.status(200).json({revalidated: true});
}

function parseSignature(value: string | string[] | undefined): {timestamp: number; digest: string} | null {
  if (typeof value !== 'string') return null;
  let timestamp = 0;
  let digest = '';
  for (const part of value.split(',')) {
    const [key, entry] = part.trim().split('=', 2);
    if (key === 't') timestamp = Number(entry);
    if (key === 'v1') digest = entry ?? '';
  }
  return Number.isSafeInteger(timestamp) && /^[a-f0-9]{64}$/.test(digest) ? {timestamp, digest} : null;
}

function safeEqual(expected: string, actual: string): boolean {
  const left = Buffer.from(expected, 'hex');
  const right = Buffer.from(actual, 'hex');
  return left.length === right.length && crypto.timingSafeEqual(left, right);
}

function validPayload(payload: RevalidationPayload): boolean {
  return typeof payload.event_id === 'string' && /^[1-9][0-9]*$/.test(payload.event_id) &&
    Array.isArray(payload.paths) && payload.paths.length > 0 && payload.paths.length <= 100 &&
    payload.paths.every((path) => typeof path === 'string' && path.startsWith('/') && !path.startsWith('//') && path.length <= 500 && !path.includes('\\') && !path.includes('?') && !path.includes('#'));
}

async function readBody(request: NextApiRequest, maxBytes: number): Promise<Buffer> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of request) {
    const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
    size += buffer.length;
    if (size > maxBytes) throw new Error('request body too large');
    chunks.push(buffer);
  }
  return Buffer.concat(chunks);
}
