import {it} from 'node:test';
import assert from 'node:assert/strict';
import {discoverControls,parseAriaSnapshot} from '../lib/discover.mjs';
const heading={tag:'h1',visible:true,name:'What changed?'};
const choices=[
  {tag:'fieldset',visible:true,class:'choice-fieldset'},
  {tag:'button',visible:true,class:'choice',name:'1 A response ↗'},
  {tag:'button',visible:true,class:'choice',name:'2 A validator ↗'}
];
const ariaHeading={role:'heading',level:1,name:'What changed?'};
it('choice affordance requires independently matching accessibility controls',()=>{
  const observation={html:'',dom:[heading,...choices],aria:[ariaHeading,{role:'button',name:'A response'},{role:'button',name:'A validator'}]};
  assert.equal(discoverControls(observation).answerAffordance.status,'confirmed');
  observation.aria=[ariaHeading];
  assert.equal(discoverControls(observation).answerAffordance.status,'unverified');
  assert.equal(discoverControls(observation).state,'unknown');
});
it('unavailable accessibility does not become missing controls or graded evidence',()=>{
  const observation={html:'',aria:[],dom:[heading,{tag:'form',visible:true,dataset:{next:''}},{tag:'div',role:'status',visible:true}]};
  assert.equal(discoverControls(observation).state,'unknown');
  assert.equal(discoverControls(observation).answerAffordance.status,'unverified');
});
it('held feedback needs visible status and deliberate Next in both observations',()=>{
  const observation={html:'',aria:[ariaHeading,{role:'status'},{role:'button',name:'Next question'}],dom:[heading,{tag:'form',visible:true,dataset:{next:''}},{tag:'div',role:'status',visible:true}]};
  assert.equal(discoverControls(observation).state,'graded');
  observation.aria=[ariaHeading,{role:'status'}];
  assert.equal(discoverControls(observation).state,'unknown');
});

it('parses inline accessible status text used by a real held-feedback surface',()=>{
  const aria=parseAriaSnapshot('- heading "What changed?" [level=1]\n- status: Answer received\n- button "Next"');
  assert.equal(aria.find(node=>node.role==='status')?.name,'Answer received');
  const observation={html:'',aria,dom:[heading,{tag:'form',visible:true,dataset:{next:''}},{tag:'div',role:'status',visible:true}]};
  assert.equal(discoverControls(observation).state,'graded');
});
