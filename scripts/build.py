#!/usr/bin/env python3
"""Build both browser frontends and a self-contained Linux Go binary locally."""
import os, shutil, subprocess
from pathlib import Path
root = Path(__file__).resolve().parents[1]
def run(args, cwd=root, env=None):
    subprocess.run(args, cwd=cwd, env=env, check=True)
run(['npm', 'test'], root / 'miniapp')
run(['npm', 'run', 'type-check'], root / 'miniapp')
run(['npm', 'run', 'build:h5'], root / 'miniapp')
run(['npm', 'run', 'build:mp-weixin'], root / 'miniapp')
run(['npm', 'run', 'build'], root / 'admin')
web = root / 'server/web'
for path in web.iterdir():
    if path.name == '.gitkeep': continue
    if path.is_dir(): shutil.rmtree(path)
    else: path.unlink()
shutil.copytree(root / 'miniapp/dist/build/h5', web, dirs_exist_ok=True)
shutil.copytree(root / 'admin/dist', web / 'admin', dirs_exist_ok=True)
run(['go', 'test', './...'], root / 'server')
run(['go', 'vet', './...'], root / 'server')
release = root / '.deploy'
release.mkdir(exist_ok=True)
run(['go', 'build', '-trimpath', '-ldflags=-s -w', '-o', str(release / 'mind-offline'), '.'], root / 'server', dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH='amd64'))
shutil.copy(root / 'deploy/Dockerfile', release / 'Dockerfile')
print('Ready: .deploy/mind-offline; miniapp/dist/build/mp-weixin')
