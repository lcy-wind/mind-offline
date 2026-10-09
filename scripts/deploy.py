#!/usr/bin/env python3
"""Deploy a prebuilt binary over existing SSH authentication, preserving last release."""
import datetime, os, shlex, subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[1]
host=os.environ.get('DEPLOY_HOST','root@82.158.89.253')
control=os.environ.get('SSH_CONTROL_PATH','/tmp/mind-offline-ssh')
ssh=['ssh']+(['-S',control] if Path(control).exists() else [])+[host]
scp=['scp']+(['-o','ControlPath='+control] if Path(control).exists() else [])
tag='mind-offline:'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d-%H%M%S')
release='/opt/mind-offline/releases/'+tag.split(':')[1]
def remote(command): subprocess.run(ssh+[command],check=True)
if not (root/'.deploy/mind-offline').exists(): raise SystemExit('Run scripts/build.py first')
remote('test -s /opt/mind-offline/production.env && install -d -m 700 '+shlex.quote(release))
subprocess.run(scp+[str(root/'.deploy/mind-offline'),str(root/'deploy/Dockerfile'),host+':'+release+'/'],check=True)
subprocess.run(scp+[str(root/'deploy/compose.yaml'),host+':/opt/mind-offline/compose.yaml'],check=True)
command='''set -eu
cd /opt/mind-offline
old_image=$(docker inspect mind-offline --format '{{.Config.Image}}' 2>/dev/null || true)
docker build -t NEW_IMAGE RELEASE
healthy=0
if MIND_OFFLINE_IMAGE=NEW_IMAGE docker compose up -d; then
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if docker exec mind-offline /mind-offline healthcheck; then healthy=1; break; fi
  sleep 2
done
fi
if [ "$healthy" = 0 ]; then
  if [ -n "$old_image" ]; then MIND_OFFLINE_IMAGE="$old_image" docker compose up -d; fi
  echo 'Health check failed; attempted rollback to previous image.' >&2
  exit 1
fi
printf 'MIND_OFFLINE_IMAGE=%s\\n' NEW_IMAGE > .env
chmod 600 .env
printf 'Healthy release: %s\\n' NEW_IMAGE
'''.replace('NEW_IMAGE',shlex.quote(tag)).replace('RELEASE',shlex.quote(release))
remote(command)
print('Deployed '+tag)
