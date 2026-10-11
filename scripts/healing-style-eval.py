#!/usr/bin/env python3
"""Opt-in sequential free-model samples; key is loaded only on the server."""
import argparse, shlex, subprocess
from pathlib import Path
parser=argparse.ArgumentParser()
parser.add_argument('--baseline',action='store_true')
parser.add_argument('--backup',action='store_true')
parser.add_argument('--case',default='')
parser.add_argument('--role',default='ENFP')
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
ssh=['ssh','-S','/tmp/mind-offline-ssh','root@82.158.89.253']
subprocess.run(ssh+['install -d -m 700 /opt/mind-offline/qa'],check=True)
subprocess.run(['scp','-o','ControlPath=/tmp/mind-offline-ssh',str(root/'.deploy/healing-style-eval'),'root@82.158.89.253:/opt/mind-offline/qa/healing-style-eval'],check=True)
remote="""from pathlib import Path
import os,subprocess
values=dict(line.split('=',1) for line in Path('/opt/mind-offline/production.env').read_text().splitlines() if '=' in line and not line.startswith('#'))
env=os.environ.copy();env['AI_API_KEY']=values['AI_API_KEY'];env['HEALING_STYLE_EVAL']='1'
"""
remote+="env['HEALING_STYLE_ROLE']="+repr(args.role)+"\n"
if args.case:remote+="env['HEALING_STYLE_CASE']="+repr(args.case)+"\n"
if args.baseline:remote+="env['HEALING_STYLE_BASELINE']='1'\n"
if args.backup:remote+="env['HEALING_STYLE_MODEL']='backup'\n"
remote+="os.chmod('/opt/mind-offline/qa/healing-style-eval',0o700)\nr=subprocess.run(['/opt/mind-offline/qa/healing-style-eval','-test.run','^TestHealingStyleLive$','-test.v','-test.timeout','20m'],env=env)\nraise SystemExit(r.returncode)\n"
raise SystemExit(subprocess.run(ssh+['python3 -c '+shlex.quote(remote)]).returncode)
