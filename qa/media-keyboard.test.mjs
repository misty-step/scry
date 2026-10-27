// Exercise the exact application handler without a browser or Worker build.
import assert from 'node:assert/strict';
import test from 'node:test';
import {readFileSync} from 'node:fs';
import {runInNewContext} from 'node:vm';
const source=readFileSync(new URL('../assets/app.js',import.meta.url),'utf8');
class Element {
 constructor(tag,parent=null){this.tag=tag;this.parent=parent;this.isContentEditable=false;}
 matches(selector){return selector.split(',').some(s=>s.trim()===this.tag||(!s.includes('#')&&s.trim()===this.tag.split('#')[0]));}
 closest(selector){return this.matches(selector)?this:this.parent?.closest(selector)||null;}
}
function keyboard(target,key){
 const handlers=new Map();let next=0,choice=0,answer=0,prevented=false;
 const nextForm={requestSubmit:()=>next++},choiceButton={form:{requestSubmit:()=>choice++}};
 target.form={requestSubmit:()=>answer++};
 const document={addEventListener:(name,fn)=>handlers.set(name,fn),querySelector:selector=>selector==='form[data-next]'?nextForm:selector==='[data-choice="1"]'?choiceButton:null};
 runInNewContext(source,{document,window:{addEventListener(){}},navigator:{onLine:true},HTMLElement:Element,clearTimeout(){},matchMedia:()=>({matches:true})});
 handlers.get('keydown')({target,key,preventDefault(){prevented=true;}});
 return {next,choice,answer,prevented};
}
for(const key of [' ','Enter']){
 test(`Focused native audio keeps ${JSON.stringify(key)} without submitting Next`,()=>{
  for(const target of [new Element('audio'),new Element('span',new Element('audio'))])assert.deepEqual(keyboard(target,key),{next:0,choice:0,answer:0,prevented:false});
 });
 test(`Unfocused Next shortcut still accepts ${JSON.stringify(key)}`,()=>assert.deepEqual(keyboard(new Element('main'),key),{next:1,choice:0,answer:0,prevented:true}));
 test(`Existing interactive controls retain ${JSON.stringify(key)}`,()=>{
  for(const tag of ['a','button','summary','input','textarea','select'])assert.deepEqual(keyboard(new Element(tag),key),{next:0,choice:0,answer:0,prevented:false});
 });
}
test('Existing numeric choice and desktop answer shortcuts remain unchanged',()=>{
 assert.deepEqual(keyboard(new Element('main'),'1'),{next:0,choice:1,answer:0,prevented:true});
 assert.deepEqual(keyboard(new Element('textarea#answer'),'Enter'),{next:0,choice:0,answer:1,prevented:true});
});
