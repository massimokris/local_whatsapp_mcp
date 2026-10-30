import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import test from "node:test";
import pg from "pg";

const landing = fileURLToPath(new URL("../", import.meta.url));
const downloadUrl = "https://github.com/massimokris/local_whatsapp_mcp/releases/download/v1.1.1/LocalWhatsAppMCP-1.1.1-arm64.dmg";

async function freePort() {
  const socket = createServer();
  socket.listen(0, "127.0.0.1");
  await once(socket, "listening");
  const port = socket.address().port;
  await new Promise((resolve) => socket.close(resolve));
  return port;
}

function command(name, args) {
  const result = spawnSync(name, args, { encoding: "utf8", timeout: 15_000 });
  assert.equal(result.status, 0, `${name} failed: ${result.error?.message || result.stderr}`);
}

async function stopServer(server) {
  if (!server || server.exitCode !== null || server.signalCode !== null) return;
  const exited = once(server, "exit");
  server.kill("SIGTERM");
  await Promise.race([exited, delay(3_000, undefined, { ref: false })]);
  if (server.exitCode === null && server.signalCode === null) {
    server.kill("SIGKILL");
    await Promise.race([exited, delay(1_000, undefined, { ref: false })]);
  }
}

test("email signups persist before the app download", { timeout: 60_000 }, async (t) => {
  const temporary = await mkdtemp(join(tmpdir(), "localwhatsapp-signups-"));
  const data = join(temporary, "postgres");
  let server;
  let database;

  t.after(async () => {
    await stopServer(server);
    await database?.end().catch(() => undefined);
    spawnSync("pg_ctl", ["-D", data, "-m", "immediate", "-w", "-t", "5", "stop"], {
      stdio: "ignore",
      timeout: 7_000,
    });
    await rm(temporary, { recursive: true, force: true });
  });

  command("initdb", ["-D", data, "-U", "signup_test", "--auth=trust", "--no-locale", "--encoding=UTF8"]);
  const databasePort = await freePort();
  command("pg_ctl", [
    "-D", data, "-l", join(temporary, "postgres.log"), "-w", "-t", "10",
    "-o", `-h 127.0.0.1 -p ${databasePort} -k ''`, "start",
  ]);

  // Every database connection uses this owned, disposable cluster.
  const databaseUrl = `postgresql://signup_test@127.0.0.1:${databasePort}/postgres`;
  database = new pg.Client({ connectionString: databaseUrl, connectionTimeoutMillis: 5_000 });
  await database.connect();
  await database.query(await readFile(join(landing, "db/schema.sql"), "utf8"));
  await database.query(`
    CREATE ROLE waitlist_writer LOGIN;
    GRANT CONNECT ON DATABASE postgres TO waitlist_writer;
    GRANT USAGE ON SCHEMA public TO waitlist_writer;
    GRANT INSERT (email, locale, source) ON public.waitlist_signups TO waitlist_writer;
    ALTER TABLE public.waitlist_signups ENABLE ROW LEVEL SECURITY;
    CREATE POLICY waitlist_writer_insert ON public.waitlist_signups
      FOR INSERT TO waitlist_writer WITH CHECK (true);
  `);
  assert.equal((await database.query(
    "SELECT has_table_privilege('waitlist_writer', 'public.waitlist_signups', 'SELECT') AS can_read",
  )).rows[0].can_read, false);
  await database.query(await readFile(join(landing, "db/append-source-permissions.sql"), "utf8"));
  assert.deepEqual((await database.query(`SELECT
    has_column_privilege('waitlist_writer', 'public.waitlist_signups', 'source', 'UPDATE') AS can_update_source,
    has_column_privilege('waitlist_writer', 'public.waitlist_signups', 'locale', 'UPDATE') AS can_update_locale,
    has_column_privilege('waitlist_writer', 'public.waitlist_signups', 'email', 'SELECT') AS can_read_email,
    has_column_privilege('waitlist_writer', 'public.waitlist_signups', 'locale', 'SELECT') AS can_read_locale
  `)).rows[0], {
    can_update_source: true, can_update_locale: false, can_read_email: true, can_read_locale: false,
  });

  const port = await freePort();
  const origin = `http://127.0.0.1:${port}`;
  let output = "";
  let serverError;
  server = spawn(process.execPath, [join(landing, "node_modules/next/dist/bin/next"), "start", "--hostname", "127.0.0.1", "--port", String(port)], {
    cwd: landing,
    env: {
      PATH: process.env.PATH,
      HOME: process.env.HOME,
      NODE_ENV: "production",
      NEXT_TELEMETRY_DISABLED: "1",
      DATABASE_URL: `postgresql://waitlist_writer@127.0.0.1:${databasePort}/postgres`,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  server.on("error", (error) => { serverError = error; });
  for (const stream of [server.stdout, server.stderr]) {
    stream.on("data", (chunk) => { output = (output + chunk).slice(-16_384); });
  }

  const deadline = Date.now() + 15_000;
  let ready = false;
  while (Date.now() < deadline && !serverError && server.exitCode === null) {
    ready = await fetch(origin, { signal: AbortSignal.timeout(500) }).then((response) => response.ok).catch(() => false);
    if (ready) break;
    await delay(100);
  }
  assert.ok(ready, `Built Next server did not start. Run npm run build first. ${serverError?.message || output}`);

  function post(body, contentType = "application/json", extraHeaders = {}) {
    return fetch(`${origin}/api/signups`, {
      method: "POST",
      headers: { "Content-Type": contentType, Origin: origin, ...extraHeaders },
      body,
      redirect: "manual",
      signal: AbortSignal.timeout(10_000),
    });
  }

  await t.test("rejects malformed, oversized and cross-origin input without storing it", async () => {
    for (const body of ["{", "null", "[]", "42", "{}", '{"email":123}', '{"email":"invalid"}', JSON.stringify({ email: `${"x".repeat(245)}@example.com` })]) {
      const response = await post(body);
      assert.equal(response.status, 400);
      assert.equal((await response.json()).ok, false);
    }
    assert.equal((await post("email=test@example.com", "text/plain")).status, 415);
    assert.equal((await post("x".repeat(2_049))).status, 413);
    assert.equal((await post('{"email":"cross-origin@example.com"}', "application/json", { Origin: "https://other.example.com" })).status, 403);
    for (const invalidOrigin of ["null", "not a URL", ""]) {
      assert.equal((await post('{"email":"invalid-origin@example.com"}', "application/json", { Origin: invalidOrigin })).status, 403);
    }
    assert.equal((await post("email=first@example.com&email=second@example.com", "application/x-www-form-urlencoded")).status, 400);
    assert.equal((await database.query("SELECT count(*)::int AS count FROM public.waitlist_signups")).rows[0].count, 0);
  });

  await t.test("normalizes and deduplicates addresses with fixed signup metadata", async () => {
    for (const email of ["  Download.Test@Example.com  ", "download.test@example.com"]) {
      const response = await post(JSON.stringify({ email, locale: "fr", source: "forged_client_source" }));
      assert.equal(response.status, 200);
      assert.deepEqual(await response.json(), { ok: true, downloadUrl });
    }
    const result = await database.query("SELECT email, locale, source, created_at FROM public.waitlist_signups");
    assert.equal(result.rowCount, 1);
    assert.equal(result.rows[0].email, "download.test@example.com");
    assert.equal(result.rows[0].locale, "en");
    assert.equal(result.rows[0].source, "landing_localwhatsappmcp");
    assert.ok(result.rows[0].created_at instanceof Date);
  });

  await t.test("supports a native form without exposing the email in the redirect", async () => {
    const response = await post(new URLSearchParams({
      email: "  NoJS.Signup@Example.com ", locale: "es", source: "forged_form_source",
    }), "application/x-www-form-urlencoded");
    assert.equal(response.status, 303);
    assert.equal(response.headers.get("location"), downloadUrl);
    assert.deepEqual((await database.query(
      "SELECT email, locale, source FROM public.waitlist_signups WHERE email = $1", ["nojs.signup@example.com"],
    )).rows, [{ email: "nojs.signup@example.com", locale: "en", source: "landing_localwhatsappmcp" }]);
  });

  await t.test("appends this source once to an existing keyboard signup and preserves other fields", async () => {
    const existing = await database.query(
      `INSERT INTO public.waitlist_signups (email, locale, source, created_at)
       VALUES ($1, $2, $3, $4) RETURNING id, email, locale, source, created_at`,
      ["existing.user@example.com", "es", "landing", "2025-01-02T03:04:05.000Z"],
    );
    for (let attempt = 0; attempt < 3; attempt += 1) {
      const response = await post(JSON.stringify({ email: "  Existing.User@Example.com  " }));
      assert.equal(response.status, 200);
      assert.deepEqual(await response.json(), { ok: true, downloadUrl });
    }
    const after = await database.query(
      "SELECT id, email, locale, source, created_at FROM public.waitlist_signups WHERE email = $1",
      ["existing.user@example.com"],
    );
    assert.deepEqual(after.rows, [{ ...existing.rows[0], source: "landing,landing_localwhatsappmcp" }]);
  });

  await t.test("matches complete source labels anywhere in the list and handles an empty source", async () => {
    const cases = [
      ["landing_localwhatsappmcp,landing", "landing_localwhatsappmcp,landing"],
      ["newsletter,landing_localwhatsappmcp,landing", "newsletter,landing_localwhatsappmcp,landing"],
      ["landing_localwhatsappmcp_extra", "landing_localwhatsappmcp_extra,landing_localwhatsappmcp"],
      ["", "landing_localwhatsappmcp"],
    ];
    for (const [index, [source, expected]] of cases.entries()) {
      const email = `source-case-${index}@example.com`;
      await database.query("INSERT INTO public.waitlist_signups (email, source) VALUES ($1, $2)", [email, source]);
      for (let attempt = 0; attempt < 2; attempt += 1) {
        assert.equal((await post(JSON.stringify({ email }))).status, 200);
      }
      assert.deepEqual((await database.query(
        "SELECT source FROM public.waitlist_signups WHERE email = $1", [email],
      )).rows, [{ source: expected }]);
    }
  });

  await t.test("concurrent repeat submissions append one source and preserve existing attribution", async () => {
    const email = "concurrent.signup@example.com";
    await database.query(
      "INSERT INTO public.waitlist_signups (email, source) VALUES ($1, $2)", [email, "newsletter,landing"],
    );
    const responses = await Promise.all(Array.from({ length: 8 }, () => post(JSON.stringify({ email }))));
    for (const response of responses) {
      assert.equal(response.status, 200);
      assert.deepEqual(await response.json(), { ok: true, downloadUrl });
    }
    assert.deepEqual((await database.query(
      "SELECT source FROM public.waitlist_signups WHERE email = $1", [email],
    )).rows, [{ source: "newsletter,landing,landing_localwhatsappmcp" }]);
  });

  await t.test("does not offer a download when storage is unavailable", async () => {
    await database.end();
    database = undefined;
    command("pg_ctl", ["-D", data, "-m", "fast", "-w", "-t", "5", "stop"]);
    const response = await post('{"email":"retry@example.com"}');
    assert.equal(response.status, 503);
    assert.equal(response.headers.get("location"), null);
    assert.deepEqual(await response.json(), { ok: false, error: "Could not save your email. Please try again." });
  });
});
