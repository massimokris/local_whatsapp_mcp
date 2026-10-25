#!/usr/bin/env python3
"""Exercise bundled HTTP pairing, SQLite credentials, and MCP without a WhatsApp account.

Uses only a disposable project build directory; no Keychain access.
Never connects WhatsApp or changes a real client config.
"""
import http.client
import json
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlparse

root = Path(__file__).resolve().parent.parent
app = Path(sys.argv[1] if len(sys.argv) > 1 else root / 'dist/LocalWhatsAppMCP.app').resolve()
helper = app / 'Contents/Helpers/localwhatsappmcp'
engine = app / 'Contents/Helpers/wacli'
(root / 'build').mkdir(exist_ok=True)
data = Path(tempfile.mkdtemp(prefix='smoke.', dir=root / 'build'))
process = None
checks = []


def check(condition, name):
    if not condition:
        raise RuntimeError(name)
    checks.append(name)


def invoke(args, body=None, succeeds=True):
    result = subprocess.run([str(helper), *args], input=body, capture_output=True, timeout=30)
    if (result.returncode == 0) != succeeds:
        # Captured tool output can contain credentials: never include it in errors.
        raise RuntimeError('unexpected helper result: ' + args[0])
    return json.loads(result.stdout) if result.returncode == 0 else None


def control(method, path, body=None, succeeds=True):
    return invoke(['control', '--data-dir', str(data), '--method', method, '--path', path],
                  json.dumps(body).encode() if body is not None else None, succeeds)


def status(url, path, host=None):
    parsed = urlparse(url)
    client = http.client.HTTPConnection(parsed.hostname, parsed.port, timeout=10)
    try:
        client.request('GET', path, headers={'Host': host or parsed.netloc})
        response = client.getresponse()
        response.read()
        return response.status
    finally:
        client.close()


def direct_rpc(credentials, method, params, request_id):
    parsed = urlparse(credentials['url'])
    client = http.client.HTTPConnection(parsed.hostname, parsed.port, timeout=15)
    try:
        client.request('POST', parsed.path, json.dumps({'jsonrpc': '2.0', 'id': request_id,
                        'method': method, 'params': params}), headers={
                            'Authorization': 'Bearer ' + credentials['access_token'],
                            'Content-Type': 'application/json',
                            'Accept': 'application/json, text/event-stream',
                            'MCP-Protocol-Version': '2025-11-25'})
        response = client.getresponse()
        return response.status, json.loads(response.read())
    finally:
        client.close()


def exchange(url, body, method='POST', headers=None):
    parsed = urlparse(url)
    client = http.client.HTTPConnection(parsed.hostname, parsed.port, timeout=15)
    try:
        request_headers = {'Content-Type': 'application/json'}
        request_headers.update(headers or {})
        client.request(method, parsed.path, json.dumps(body), headers=request_headers)
        response = client.getresponse()
        payload = response.read()
        value = json.loads(payload) if response.getheader('Content-Type', '').startswith('application/json') else None
        return response.status, value, response.getheader('Cache-Control', '')
    finally:
        client.close()


def rpc(credentials, method, params, request_id):
    code, value = direct_rpc(credentials, method, params, request_id)
    if code != 200 or 'error' in value or 'result' not in value:
        raise RuntimeError('MCP request failed: ' + method)
    return value['result']


try:
    process = subprocess.Popen([str(helper), 'serve', '--data-dir', str(data),
                                '--wacli', str(engine),
                                '--listen', '127.0.0.1:0', '--owner-stdin'],
                               stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    deadline = time.monotonic() + 20
    while not (data / 'control.sock').exists() and process.poll() is None and time.monotonic() < deadline:
        time.sleep(0.1)
    check(process.poll() is None and (data / 'control.sock').exists(), 'bundled service starts with private control')
    state = control('GET', '/state')
    credentials = data / 'credentials.db'
    check(credentials.is_file() and credentials.stat().st_mode & 0o777 == 0o600,
          'credential database is stored locally with owner-only permissions')
    with sqlite3.connect(credentials) as secrets_db:
        check(secrets_db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok',
              'local credential database passes SQLite integrity check')
    check(state['scope']['mode'] == 'initial' and state['mcp']['status'] == 'setup_required', 'initial capture keeps MCP unavailable')
    endpoint = state['mcp']['url']
    check(urlparse(endpoint).hostname == '127.0.0.1', 'MCP endpoint is restricted to loopback')
    check('cloud' not in state, 'service state exposes only local connection settings')
    check(status(endpoint, '/mcp') == 401, 'local MCP rejects unauthenticated requests')
    check(status(endpoint, '/state') == 404, 'private controls are absent from MCP listener')
    check(status(endpoint, '/mcp', 'untrusted.invalid') == 403, 'MCP listener rejects unrecognized hosts')
    retired_routes = ['/.well-known/oauth-authorization-server', '/.well-known/oauth-protected-resource',
                      '/.well-known/oauth-protected-resource/mcp', '/oauth/bootstrap', '/oauth/ready',
                      '/oauth/authorize', '/oauth/consent', '/oauth/register', '/oauth/token']
    check(all(status(endpoint, path) == 404 for path in retired_routes), 'OAuth and external authorization routes are absent')
    control('POST', '/cloud/instructions', {'provider': 'ngrok', 'name': 'Retired cloud setup'}, succeeds=False)
    check(True, 'external agent setup is unavailable')
    control('POST', '/pairing', {'name': 'Disposable smoke'}, succeeds=False)
    control('POST', '/agents/manual', {'name': 'Disposable manual'}, succeeds=False)
    check(True, 'manual connection creation waits for confirmed scope')
    control('PUT', '/scope', {'mode': 'selected', 'chat_ids': ['123@s.whatsapp.net']})
    initialized_store = subprocess.run([str(engine), '--store', str(data / 'store'),
                                        '--content-policy', str(data / 'content-policy.json'),
                                        '--json', 'auth', 'status'], capture_output=True, timeout=30)
    check(initialized_store.returncode == 0, 'pinned engine initializes an unlinked synthetic archive')
    # Synthetic fixture insertion only. Production archive writes remain upstream-owned.
    with sqlite3.connect(data / 'store/wacli.db') as archive:
        archive.executemany('INSERT INTO chats(jid,kind,name,last_message_ts,archived) VALUES(?,?,?,?,?)',
                            [('123@s.whatsapp.net', 'dm', 'Fixture Work', 1003, 1),
                             ('999@s.whatsapp.net', 'dm', 'Fixture Hidden', 1004, 1)])
        archive.executemany('INSERT INTO messages(chat_jid,msg_id,ts,from_me,text,display_text) VALUES(?,?,?,?,?,?)',
                            [('123@s.whatsapp.net', 'm' + str(i), 1000 + i, 0, 'alpha fixture ' + str(i), 'alpha fixture ' + str(i))
                             for i in range(1, 4)] +
                            [('999@s.whatsapp.net', 'hidden1', 1004, 0, 'alpha secret fixture', 'alpha secret fixture')])
    manual = control('POST', '/agents/manual', {'name': 'Disposable manual'})
    details = control('GET', '/agents/' + manual['id'] + '/credentials')
    check(details == manual and manual['url'] == endpoint + '/mcp',
          'manual connection details reopen with the same URL and access token')
    check(manual['access_token'] not in json.dumps(control('GET', '/state')),
          'normal service state omits the manual access token')
    code, initialized_manual = direct_rpc(manual, 'initialize', {
        'protocolVersion': '2025-11-25', 'capabilities': {},
        'clientInfo': {'name': 'manual-bundle-smoke', 'version': '1'}}, 1)
    check(code == 200 and initialized_manual.get('result', {}).get('serverInfo', {}).get('name') == 'LocalWhatsAppMCP',
          'manually configured URL and bearer initialize directly over HTTP')
    code, manual_messages = direct_rpc(manual, 'tools/call', {
        'name': 'read_messages', 'arguments': {'chat_id': '123@s.whatsapp.net', 'limit': 10}}, 2)
    result = manual_messages.get('result', {})
    check(code == 200 and not result.get('isError', False)
          and len(result.get('structuredContent', {}).get('messages', [])) == 3
          and 'secret fixture' not in json.dumps(result),
          'direct manual bearer reads only selected synthetic messages')
    invitation = control('POST', '/pairing', {'name': 'Disposable smoke'})
    exchange_url = endpoint + '/pairing/redeem'
    check(exchange_url in invitation['instructions'] and invitation['token'] in invitation['instructions']
          and str(helper) not in invitation['instructions'],
          'generic instructions contain the HTTP exchange endpoint and temporary pairing code')
    code, _, _ = exchange(exchange_url, {'token': invitation['token']}, method='GET')
    check(code == 405, 'HTTP pairing rejects GET without consuming the invitation')
    code, _, _ = exchange(exchange_url, {'token': invitation['token']}, headers={'Host': 'untrusted.invalid'})
    check(code == 403, 'HTTP pairing rejects unrecognized hosts without consuming the invitation')
    code, _, _ = exchange(exchange_url, {'token': invitation['token']}, headers={'Origin': 'https://untrusted.invalid'})
    check(code == 403, 'HTTP pairing rejects foreign origins without consuming the invitation')
    code, _, _ = exchange(exchange_url, {})
    check(400 <= code < 500, 'HTTP pairing rejects a missing temporary code')
    code, paired, cache_control = exchange(exchange_url, {'token': invitation['token']})
    check(code == 200 and paired['url'] == endpoint + '/mcp' and paired['token_type'] == 'Bearer'
          and paired['access_token'] != invitation['token'] and paired['id'],
          'single-use HTTP exchange returns a durable bearer and complete MCP URL')
    check('no-store' in cache_control, 'HTTP pairing response prevents credential caching')
    connection = paired['id']
    paired_state = control('GET', '/state')
    grant = next(agent for agent in paired_state['agents'] if agent['id'] == connection)
    check(grant['pairing_invitation_id'] == invitation['invitation_id'] and not grant['send_enabled']
          and paired['access_token'] not in json.dumps(paired_state),
          'HTTP pairing records the invitation identity, starts read-only, and keeps credentials out of state')
    paired_details = control('GET', '/agents/' + connection + '/credentials')
    check(paired_details == {'id': connection, 'name': grant['name'], 'url': paired['url'],
                            'access_token': paired['access_token']},
          'private credentials for an HTTP-paired connection return its original URL and bearer')
    after_details = control('GET', '/state')
    after_grant = next(agent for agent in after_details['agents'] if agent['id'] == connection)
    check(after_grant == grant and not after_grant['send_enabled']
          and len(after_details['agents']) == len(paired_state['agents'])
          and paired['access_token'] not in json.dumps(after_details),
          'opening paired credentials preserves the grant, disabled sending, and redacted state')
    code, _, _ = exchange(endpoint + '/agents/' + connection + '/credentials', {}, method='GET',
                          headers={'Authorization': 'Bearer ' + paired['access_token']})
    check(code == 404, 'paired credentials remain private even to an authenticated HTTP client')
    code, _, _ = exchange(exchange_url, {'token': invitation['token']})
    check(code == 401, 'pairing invitation cannot be redeemed twice over HTTP')
    initialized = rpc(paired, 'initialize', {'protocolVersion': '2025-11-25', 'capabilities': {},
                                            'clientInfo': {'name': 'disposable-bundle-smoke', 'version': '1'}}, 1)
    check(initialized['serverInfo']['name'] == 'LocalWhatsAppMCP', 'HTTP-paired bearer initializes MCP directly')
    listing = rpc(paired, 'tools/list', {}, 2)
    check(len(listing['tools']) == 9, 'HTTP-paired bearer discovers all nine MCP tools')
    tool_status = rpc(paired, 'tools/call', {'name': 'check_status', 'arguments': {}}, 3)
    check(not tool_status.get('isError', False), 'HTTP-paired bearer calls authenticated status')
    def tool(name, arguments, request_id):
        result = rpc(paired, 'tools/call', {'name': name, 'arguments': arguments}, request_id)
        if result.get('isError', False):
            raise RuntimeError('tool failed against synthetic archive: ' + name)
        encoded = json.dumps(result)
        check('999@s.whatsapp.net' not in encoded and 'secret fixture' not in encoded,
              name + ' excludes unselected synthetic content')
        return result['structuredContent']
    conversations = tool('find_conversations', {}, 10)
    check([row['id'] for row in conversations['chats']] == ['123@s.whatsapp.net'], 'real archive catalog respects scope')
    check(conversations['chats'][0].get('archived') is True, 'conversation results expose the stored WhatsApp archived status')
    with sqlite3.connect(data / 'store/wacli.db') as archive:
        archive.execute('UPDATE chats SET archived=0 WHERE jid=?', ('123@s.whatsapp.net',))
    conversations = tool('find_conversations', {}, 101)
    check(conversations['chats'][0].get('archived') is False, 'conversation results reflect unarchive without omitting false')
    first = tool('read_messages', {'chat_id': '123@s.whatsapp.net', 'limit': 2}, 11)
    check(len(first['messages']) == 2 and first['has_more'], 'real archive message pagination is bounded')
    second = tool('read_messages', {'chat_id': '123@s.whatsapp.net', 'limit': 2, 'cursor': first['next_cursor']}, 12)
    message_ids = [row['message_id'] for row in first['messages'] + second['messages']]
    check(len(message_ids) == 3 and len(set(message_ids)) == 3 and not second['has_more'], 'message cursor resumes without duplicate rows')
    found = tool('search_messages', {'query': 'alpha'}, 13)
    check(len(found['messages']) == 3, 'real FTS5 search returns selected matches')
    context = tool('read_context', {'chat_id': '123@s.whatsapp.net', 'message_id': 'm2', 'before': 1, 'after': 1}, 14)
    check([row['message_id'] for row in context['messages']] == ['m1', 'm2', 'm3'], 'real context preserves chronological neighbors')
    updates = tool('get_updates', {}, 15)
    check(len(updates['changes']) == 3 and updates['next_checkpoint'], 'real incremental feed filters hidden references')
    replay = tool('get_updates', {'checkpoint': updates['next_checkpoint']}, 16)
    check(not replay['changes'], 'incremental checkpoint resumes without replaying processed changes')
    denied = rpc(paired, 'tools/call', {'name': 'send_text', 'arguments': {'chat_id': '123@s.whatsapp.net',
                                                                'text': 'synthetic fixture', 'request_id': 'smoke-send'}}, 4)
    check(denied.get('isError', False), 'sending remains disabled without dispatch')
    before_disconnect = control('GET', '/state')
    disconnected = control('POST', '/whatsapp/disconnect')
    check(disconnected['whatsapp']['status'] == 'disconnected' and not disconnected['whatsapp'].get('linked')
          and disconnected['scope'] == before_disconnect['scope']
          and {grant['id'] for grant in disconnected['agents']} == {grant['id'] for grant in before_disconnect['agents']},
          'disconnect preserves saved scope and agent grants in the actual bundle')
    awakened = control('POST', '/lifecycle/wake')
    check(awakened['whatsapp']['status'] == 'disconnected' and not awakened['whatsapp'].get('qr'),
          'wake respects intentional disconnect without starting linking')
    retained = tool('read_messages', {'chat_id': '123@s.whatsapp.net', 'limit': 10}, 19)
    check(len(retained['messages']) == 3, 'authorized archived messages remain readable after disconnect')
    control('PUT', '/scope', {'mode': 'selected', 'chat_ids': []})
    removed = rpc(paired, 'tools/call', {'name': 'read_messages', 'arguments': {'chat_id': '123@s.whatsapp.net'}}, 17)
    old_checkpoint = rpc(paired, 'tools/call', {'name': 'get_updates', 'arguments': {'checkpoint': updates['next_checkpoint']}}, 18)
    check(removed.get('isError', False) and old_checkpoint.get('isError', False), 'scope removal denies reads and invalidates old checkpoints')
    control('DELETE', '/agents/' + connection)
    code, _ = direct_rpc(paired, 'tools/list', {}, 20)
    control('GET', '/agents/' + connection + '/credentials', succeeds=False)
    check(code == 401, 'revocation immediately denies the HTTP-paired bearer and its credentials')
    control('DELETE', '/agents/' + manual['id'])
    code, _ = direct_rpc(manual, 'tools/list', {}, 3)
    control('GET', '/agents/' + manual['id'] + '/credentials', succeeds=False)
    check(code == 401, 'revocation immediately denies the direct manual bearer and its details')
    process.stdin.close()
    process.wait(timeout=15)
    check(process.returncode == 0 and not (data / 'control.sock').exists(), 'owner pipe closure stops the service and removes control socket')
    process = None
finally:
    if process is not None and process.poll() is None:
        try:
            process.terminate()
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        except ProcessLookupError:
            pass
    shutil.rmtree(data)

print('Bundle smoke passed: ' + str(len(checks)) + ' checks; temporary SQLite data removed.')
for name in checks:
    print('PASS ' + name)
