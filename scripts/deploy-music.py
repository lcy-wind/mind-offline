#!/usr/bin/env python3
"""Deploy the loopback-only music adapter using existing SSH authentication."""
import datetime,os,shlex,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[1]
host=os.environ.get('DEPLOY_HOST','root@82.158.89.253');control=os.environ.get('SSH_CONTROL_PATH','/tmp/mind-offline-ssh')
ssh=['ssh']+(['-S',control] if Path(control).exists() else [])+[host]
scp=['scp']+(['-o','ControlPath='+control] if Path(control).exists() else [])
tag='mind-offline-music:'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d-%H%M%S')
release='/opt/mind-offline/music/releases/'+tag.split(':')[1]
subprocess.run(ssh+['test -s /opt/mind-offline/music/music.env && install -d -m 700 '+shlex.quote(release)],check=True)
files=['Dockerfile','package.json','package-lock.json','server.cjs','playback.cjs','kugou.cjs']
subprocess.run(scp+[str(root/'music-bridge'/f) for f in files]+[host+':'+release+'/'],check=True)
subprocess.run(scp+[str(root/'deploy/music-compose.yaml'),host+':/opt/mind-offline/music/compose.yaml'],check=True)
command='''set -eu
cd /opt/mind-offline/music
old_image=$(docker inspect mind-offline-music --format '{{.Config.Image}}' 2>/dev/null || true)
docker build -t NEW_TAG RELEASE
healthy=0
if MUSIC_BRIDGE_IMAGE=NEW_TAG docker compose -p mind-offline-music up -d; then
for attempt in 1 2 3 4 5 6 7 8 9 10; do
 if docker exec mind-offline-music node -e "fetch('http://127.0.0.1:18085/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"; then healthy=1; break; fi
 sleep 2
done
fi
if [ "$healthy" = 0 ]; then
 if [ -n "$old_image" ]; then MUSIC_BRIDGE_IMAGE="$old_image" docker compose -p mind-offline-music up -d; fi
 exit 1
fi
printf 'MUSIC_BRIDGE_IMAGE=%s\\n' NEW_TAG > .env
chmod 600 .env
'''.replace('NEW_TAG',shlex.quote(tag)).replace('RELEASE',shlex.quote(release))
subprocess.run(ssh+[command],check=True)
print('Healthy music bridge release: '+tag)
