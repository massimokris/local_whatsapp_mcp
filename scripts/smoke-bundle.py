#!/usr/bin/env python3
"""Exercise a bundled service/Keychain/MCP bridge without a WhatsApp account.

Uses a disposable project build directory and exact, temporary Keychain items.
Never opens provider login, connects WhatsApp, or changes a real client config.
"""
import hashlib
import http.client
import json
import os
from pathlib import Path
import select
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
bridge = None
connection_ids = set()
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


def rpc(method, params, request_id):
    payload = {'jsonrpc': '2.0', 'method': method, 'params': params}
    if request_id is not None:
        payload['id'] = request_id
    bridge.stdin.write(json.dumps(payload).encode() + b'\n')
    bridge.stdin.flush()
    if request_id is None:
        return None
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        if not select.select([bridge.stdout], [], [], 0.2)[0]:
            continue
        line = bridge.stdout.readline()
        if not line:
            break
        value = json.loads(line)
        if value.get('id') == request_id:
            if 'error' in value:
                raise RuntimeError('MCP request failed: ' + method)
            return value['result']
    raise RuntimeError('MCP request timed out: ' + method)


try:
    process = subprocess.Popen([str(helper), 'serve', '--data-dir', str(data),
                                '--wacli', str(engine), '--helper', str(helper),
                                '--listen', '127.0.0.1:0', '--owner-stdin'],
                               stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    deadline = time.monotonic() + 20
    while not (data / 'control.sock').exists() and process.poll() is None and time.monotonic() < deadline:
        time.sleep(0.1)
    check(process.poll() is None and (data / 'control.sock').exists(), 'bundled service starts with private control')
    state = control('GET', '/state')
    check(state['scope']['mode'] == 'initial' and state['mcp']['status'] == 'setup_required', 'initial capture keeps MCP unavailable')
    endpoint = state['mcp']['url']
    check(status(endpoint, '/mcp') == 401, 'public MCP rejects unauthenticated requests')
    check(status(endpoint, '/state') == 404, 'private controls are absent from public listener')
    check(status(endpoint, '/mcp', 'untrusted.invalid') == 403, 'public listener rejects unrecognized hosts')
    control('POST', '/pairing', {'name': 'Disposable smoke', 'client': 'stdio'}, succeeds=False)
    control('PUT', '/scope', {'mode': 'selected', 'chat_ids': ['123@s.whatsapp.net']})
    initialized_store = subprocess.run([str(engine), '--store', str(data / 'store'),
                                        '--content-policy', str(data / 'content-policy.json'),
                                        '--json', 'auth', 'status'], capture_output=True, timeout=30)
    check(initialized_store.returncode == 0, 'pinned engine initializes an unlinked synthetic archive')
    # Synthetic fixture insertion only. Production archive writes remain upstream-owned.
    with sqlite3.connect(data / 'store/wacli.db') as archive:
        archive.executemany('INSERT INTO chats(jid,kind,name,last_message_ts) VALUES(?,?,?,?)',
                            [('123@s.whatsapp.net', 'dm', 'Fixture Work', 1003),
                             ('999@s.whatsapp.net', 'dm', 'Fixture Hidden', 1004)])
        archive.executemany('INSERT INTO messages(chat_jid,msg_id,ts,from_me,text,display_text) VALUES(?,?,?,?,?,?)',
                            [('123@s.whatsapp.net', 'm' + str(i), 1000 + i, 0, 'alpha fixture ' + str(i), 'alpha fixture ' + str(i))
                             for i in range(1, 4)] +
                            [('999@s.whatsapp.net', 'hidden1', 1004, 0, 'alpha secret fixture', 'alpha secret fixture')])
    invitation = control('POST', '/pairing', {'name': 'Disposable smoke', 'client': 'stdio'})
    installed = invoke(['setup', '--data-dir', str(data), '--client', 'stdio', '--token-stdin'], invitation['token'].encode())
    connection_ids.add(installed['connection_id'])
    connection = installed['connection_id']
    check(installed['status'] == 'installed_and_verified', 'helper stores credential in Keychain and verifies authenticated HTTP')
    invoke(['setup', '--data-dir', str(data), '--client', 'stdio', '--token-stdin'], invitation['token'].encode(), succeeds=False)
    check(True, 'pairing invitation cannot be redeemed twice')
    bridge = subprocess.Popen([str(helper), 'stdio', '--data-dir', str(data), '--connection', connection],
                              stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, bufsize=0)
    initialized = rpc('initialize', {'protocolVersion': '2025-11-25', 'capabilities': {},
                                     'clientInfo': {'name': 'disposable-bundle-smoke', 'version': '1'}}, 1)
    check(initialized['serverInfo']['name'] == 'LocalWhatsAppMCP stdio adapter', 'stdio adapter initializes')
    rpc('notifications/initialized', {}, None)
    listing = rpc('tools/list', {}, 2)
    check(len(listing['tools']) == 9, 'stdio adapter discovers all nine MCP tools')
    tool_status = rpc('tools/call', {'name': 'check_status', 'arguments': {}}, 3)
    check(not tool_status.get('isError', False), 'stdio adapter forwards authenticated status')
    def tool(name, arguments, request_id):
        result = rpc('tools/call', {'name': name, 'arguments': arguments}, request_id)
        if result.get('isError', False):
            raise RuntimeError('tool failed against synthetic archive: ' + name)
        encoded = json.dumps(result)
        check('999@s.whatsapp.net' not in encoded and 'secret fixture' not in encoded,
              name + ' excludes unselected synthetic content')
        return result['structuredContent']
    conversations = tool('find_conversations', {}, 10)
    check([row['id'] for row in conversations['chats']] == ['123@s.whatsapp.net'], 'real archive catalog respects scope')
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
    denied = rpc('tools/call', {'name': 'send_text', 'arguments': {'chat_id': '123@s.whatsapp.net',
                                                                'text': 'synthetic fixture', 'request_id': 'smoke-send'}}, 4)
    check(denied.get('isError', False), 'sending remains disabled without dispatch')
    control('PUT', '/scope', {'mode': 'selected', 'chat_ids': []})
    removed = rpc('tools/call', {'name': 'read_messages', 'arguments': {'chat_id': '123@s.whatsapp.net'}}, 17)
    old_checkpoint = rpc('tools/call', {'name': 'get_updates', 'arguments': {'checkpoint': updates['next_checkpoint']}}, 18)
    check(removed.get('isError', False) and old_checkpoint.get('isError', False), 'scope removal denies reads and invalidates old checkpoints')
    bridge.stdin.close()
    bridge.wait(timeout=10)
    bridge = None
    control('DELETE', '/agents/' + connection)
    invoke(['doctor', '--data-dir', str(data), '--connection', connection], succeeds=False)
    check(True, 'revocation immediately denies the installed credential')
    process.stdin.close()
    process.wait(timeout=15)
    check(process.returncode == 0 and not (data / 'control.sock').exists(), 'owner pipe closure stops the service and removes control socket')
    process = None
finally:
    for child in (bridge, process):
        if child is not None and child.poll() is None:
            child.terminate()
            try:
                child.wait(timeout=15)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait(timeout=5)
    # Include credentials saved before a failed setup returned its connection ID.
    for record in (data / 'connections').glob('*.json'):
        connection_ids.add(record.stem)
    items = [('com.localwhatsappmcp.cursor', hashlib.sha256(str(data).encode()).hexdigest())]
    items.extend(('com.localwhatsappmcp.agent', item) for item in connection_ids)
    for service, account in items:
        subprocess.run(['/usr/bin/security', 'delete-generic-password', '-s', service, '-a', account],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)
    shutil.rmtree(data)

print('Bundle smoke passed: ' + str(len(checks)) + ' checks; temporary data and Keychain items removed.')
for name in checks:
    print('PASS ' + name)
