import { Client } from "pg";
import { DOWNLOAD_URL } from "../../site";

export const runtime = "nodejs";

const MAX_BODY_BYTES = 2_048;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const RETRY_MESSAGE = "Could not save your email. Please try again.";

function errorResponse(error: string, status: number) {
  return Response.json(
    { ok: false, error },
    { status, headers: { "Cache-Control": "no-store" } },
  );
}

async function readBody(request: Request): Promise<string | null> {
  if (Number(request.headers.get("content-length")) > MAX_BODY_BYTES) return null;

  const reader = request.body?.getReader();
  if (!reader) return "";

  const chunks: Uint8Array[] = [];
  let length = 0;

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > MAX_BODY_BYTES) {
        await reader.cancel();
        return null;
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const body = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder("utf-8", { fatal: true }).decode(body);
}

export async function POST(request: Request) {
  const origin = request.headers.get("origin");
  if (origin !== null) {
    try {
      const source = new URL(origin);
      // Next may reconstruct request.url using the server's bind hostname.
      if (
        !["http:", "https:"].includes(source.protocol) ||
        source.origin !== origin ||
        source.host !== request.headers.get("host")
      ) {
        return errorResponse("Invalid request origin.", 403);
      }
    } catch {
      return errorResponse("Invalid request origin.", 403);
    }
  }

  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  const isForm = contentType === "application/x-www-form-urlencoded";
  if (contentType !== "application/json" && !isForm) {
    return errorResponse("Unsupported request format.", 415);
  }

  let value: unknown;
  try {
    const body = await readBody(request);
    if (body === null) return errorResponse("Request is too large.", 413);

    if (isForm) {
      const form = new URLSearchParams(body);
      if (form.getAll("email").length !== 1) return errorResponse("Enter a valid email.", 400);
      value = form.get("email");
    } else {
      const payload: unknown = JSON.parse(body);
      if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
        return errorResponse("Invalid request.", 400);
      }
      value = (payload as Record<string, unknown>).email;
    }
  } catch {
    return errorResponse("Invalid request.", 400);
  }

  const email = typeof value === "string" ? value.trim().toLowerCase() : "";
  if (!email || email.length > 254 || !EMAIL_PATTERN.test(email)) {
    return errorResponse("Enter a valid email.", 400);
  }

  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) return errorResponse(RETRY_MESSAGE, 503);

  let client: Client | undefined;
  try {
    client = new Client({
      connectionString: databaseUrl,
      connectionTimeoutMillis: 5_000,
      query_timeout: 5_000,
      statement_timeout: 5_000,
      application_name: "local_whatsapp_mcp_landing",
    });
    client.on("error", () => {});
    await client.connect();
    await client.query(
      "INSERT INTO public.waitlist_signups (email, locale, source) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
      [email, "en", "landing_localwhatsappmcp"],
    );
  } catch {
    return errorResponse(RETRY_MESSAGE, 503);
  } finally {
    await client?.end().catch(() => undefined);
  }

  if (isForm) {
    return new Response(null, {
      status: 303,
      headers: { Location: DOWNLOAD_URL, "Cache-Control": "no-store" },
    });
  }

  return Response.json(
    { ok: true, downloadUrl: DOWNLOAD_URL },
    { headers: { "Cache-Control": "no-store" } },
  );
}
