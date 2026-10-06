"""Safe temporary installation; never touch Android data or host networking."""
import json
import os
import pathlib
import subprocess
import sys
import tempfile

data = json.load(sys.stdin)
root = pathlib.Path(data['root'])
if not root.exists() and len(data['root']) > 2 and data['root'][1] == ':':
    root = pathlib.Path('/mnt/' + data['root'][0].lower() + data['root'][2:])

with tempfile.TemporaryDirectory() as directory:
    fixture = pathlib.Path(directory)
    service = fixture / 'state/scripts/tailscaled.service'
    service.parent.mkdir(parents=True)
    service.write_text('#!/bin/sh\nprintf "service:"\nprintf "<%s>" "$@"\nprintf "\\n"\n')
    service.chmod(0o755)
    binary = fixture / 'state/bin/tailscale'
    binary.parent.mkdir()
    binary.write_text('#!/bin/sh\nprintf "cli:"\nprintf "<%s>" "$@"\nprintf "\\n"\n')
    binary.chmod(0o755)
    settings = fixture / 'state/settings.ini'
    settings.write_text(f'tailscale_bin="{binary}"\ntailscale_bin_param="--socket={fixture}/custom.sock"\n')
    wrapper = fixture / 'module/system/bin/tailscale'
    wrapper.parent.mkdir(parents=True)
    wrapper.write_text((root / 'system/bin/tailscale').read_text().replace('#!/system/bin/sh', '#!/bin/sh').replace('/data/adb/tailscale/settings.ini', str(settings)))
    wrapper.chmod(0o755)
    poison = fixture / 'poison'
    poison.mkdir()
    for name in ('tailscaled.service', 'tailscale'):
        path = poison / name
        path.write_text('#!/bin/sh\necho UNRELATED_PATH_COMMAND >&2\nexit 99\n')
        path.chmod(0o755)

    # The pre-fix root shell reproduces the exact class of failure in the phone
    # screenshot even though correctly installed files exist in the data dir.
    missing = subprocess.run(['/bin/sh', '-c', 'tailscaled.service webstatus'], env={'PATH': '/usr/bin:/bin'}, capture_output=True, text=True)
    assert missing.returncode == 127, missing
    assert 'not found' in missing.stderr, missing.stderr
    for environment in ('minimal', 'empty', 'poisoned'):
        inherited = {'minimal': '/usr/bin:/bin', 'empty': '', 'poisoned': f'{poison}:/usr/bin:/bin'}[environment]
        for item in data['commands']:
            # Map only physical installation locations into the isolated fixture.
            # /system/bin supplies ordinary Android utilities in production.
            command = item['native'].replace('/data/adb/tailscale/scripts/tailscaled.service', str(service)).replace('/data/adb/modules/tailscaled/system/bin/tailscale', str(wrapper)).replace('/system/bin:', '/usr/bin:')
            result = subprocess.run(['/bin/sh', '-c', command], env={'PATH': inherited}, capture_output=True, text=True)
            assert result.returncode == 0, f'{environment}: {item["logical"]}: exit {result.returncode}: {result.stderr}'
            assert 'UNRELATED_PATH_COMMAND' not in result.stderr, result.stderr
            if item['logical'].startswith('tailscaled.service '):
                arguments = item['logical'].split()[1:]
                assert result.stdout.strip() == 'service:' + ''.join(f'<{arg}>' for arg in arguments), result.stdout
            else:
                assert result.stdout.strip() == f'cli:<--socket={fixture}/custom.sock><up><--timeout=8s>', result.stdout
    print(f'PASS missing-PATH reproduction; {len(data["commands"])} commands in minimal, empty and poisoned PATH; real CLI wrapper preserves custom socket')
