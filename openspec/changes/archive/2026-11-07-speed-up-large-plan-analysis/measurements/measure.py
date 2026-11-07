import json,sys,time,urllib.request,pathlib
name=sys.argv[1]
root=pathlib.Path('/tmp/guit-speed-acceptance')
plan=pathlib.Path('/home/vladimir/myGit-repositories/gentoo-update-intelligence/testdata/parser/ebuild-updates-2.txt').read_text()
req=urllib.request.Request('http://127.0.0.1:18951/api/v1/analyses',data=json.dumps({'plan':plan}).encode(),headers={'Content-Type':'application/json'})
t0=time.monotonic()
with urllib.request.urlopen(req) as r: initial=json.load(r)
(root/(name+'-initial.json')).write_text(json.dumps(initial,indent=2))
observations=[]
while True:
 with urllib.request.urlopen('http://127.0.0.1:18951/api/v1/analyses/'+initial['id']) as r: result=json.load(r)
 elapsed=time.monotonic()-t0
 observations.append({'elapsed_seconds':round(elapsed,3),'status':result['status'],'progress':result.get('progress')})
 if result['status'] in ('completed','failed'):
  (root/(name+'-result.json')).write_text(json.dumps(result,indent=2,ensure_ascii=False))
  (root/(name+'-progress.json')).write_text(json.dumps(observations,indent=2))
  print(name,round(elapsed,3),result['status'],result.get('progress'),flush=True)
  break
 time.sleep(2)
