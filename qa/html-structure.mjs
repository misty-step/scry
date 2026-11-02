// Standard-library HTML parsing for Worker preflight when no browser is
// available. This proves response structure, not CSS geometry or browser UX.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
export function htmlStructure(html){
 const parsed=spawnSync('python3',['-c',String.raw`
import sys,json
from html.parser import HTMLParser
class Tree(HTMLParser):
 def __init__(self):
  super().__init__(convert_charrefs=True);self.nodes=[]
 def handle_starttag(self,tag,attrs):
  self.nodes.append({'tag':tag,'attrs':dict(attrs)})
 def handle_startendtag(self,tag,attrs):self.handle_starttag(tag,attrs)
p=Tree();p.feed(sys.stdin.read());p.close();print(json.dumps(p.nodes))
`],{input:html,encoding:'utf8',maxBuffer:4*1024*1024});
 assert.equal(parsed.status,0,parsed.stderr);const nodes=JSON.parse(parsed.stdout);
 const forms=action=>nodes.filter(n=>n.tag==='form'&&n.attrs.action===action);
 const classes=name=>nodes.filter(n=>(n.attrs.class||'').split(/\s+/).includes(name));
 return {nodes,forms,classes,withAttr:name=>nodes.filter(n=>name in n.attrs)};
}
