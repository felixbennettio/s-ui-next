"""Exercise the packaged Linux binary using a disposable runner database."""
import concurrent.futures
import http.cookiejar
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

binary = str(Path(sys.argv[1]).resolve())
subprocess.run([binary, '-v'], check=True, timeout=15)
with tempfile.TemporaryDirectory(prefix='s-ui-smoke-') as directory:
    env = {**os.environ, 'SUI_DB_FOLDER': directory, 'container': 'true'}
    subprocess.run([binary, 'setting', '-port', '22095', '-path', '/ci/',
                    '-subPort', '22096', '-subPath', '/sub/'],
                   env=env, check=True, stdout=subprocess.DEVNULL, timeout=15)
    with open(Path(directory) / 'server.log', 'w') as log:
        process = subprocess.Popen([binary], env=env, stdout=log, stderr=log)
        try:
            base = 'http://127.0.0.1:22095/ci/'
            for attempt in range(40):
                try:
                    with urllib.request.urlopen(base, timeout=2) as response:
                        html = response.read().decode()
                    break
                except (urllib.error.URLError, TimeoutError):
                    if process.poll() is not None:
                        raise RuntimeError('packaged server exited during startup')
                    time.sleep(0.25)
            else:
                raise RuntimeError('packaged web frontend did not become ready')
            scripts = re.findall(r'<script[^>]+src="([^"]+)"', html)
            assert scripts, 'embedded frontend entry point is missing'
            for script in scripts:
                with urllib.request.urlopen(urllib.parse.urljoin(base, script), timeout=5) as response:
                    assert response.status == 200 and len(response.read()) > 1000

            opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            def request(path, values=None):
                body = urllib.parse.urlencode(values).encode() if values is not None else None
                with opener.open(base + 'api/' + path, body, timeout=15) as response:
                    message = json.load(response)
                assert message.get('success') is True, 'packaged API failed: ' + path
                return message.get('obj')

            # These are disposable first-run credentials, never user credentials.
            request('login', {'user': 'admin', 'pass': 'admin'})
            with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                queries = [pool.submit(request, 'status?r=sbd') for _ in range(20)]
                for _ in range(3):
                    request('restartSb', {})
                for query in queries:
                    query.result()
            assert request('status?r=sbd')['sbd']['running']

            def save(obj, action, data):
                return request('save', {'object': obj, 'action': action,
                                       'data': json.dumps(data), 'apply': 'true'})
            save('inbounds', 'new', {'type': 'mixed', 'tag': 'ci-source',
                                    'listen': '127.0.0.1', 'listen_port': 0})
            config = request('config')['config']
            config.setdefault('route', {})['rules'] = [{'inbound': ['ci-source'], 'action': 'reject'}]
            config.setdefault('dns', {})['rules'] = [{'inbound': ['ci-source'], 'action': 'reject'}]
            save('config', 'set', config)
            updated = save('inbounds', 'del', 'ci-source')
            assert 'ci-source' not in json.dumps(updated['config'])
            assert request('status?r=sbd')['sbd']['running']
            print('Packaged binary passed: embedded assets, login, concurrent status/restarts, config save and inbound cleanup.')
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
