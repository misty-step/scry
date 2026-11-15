import assert from 'node:assert/strict';

// Journey walks, scoped to synthetic browser/policy mechanics. Live provider
// usefulness, production, real phone and independent hosted recovery
// acceptance remain explicitly unverified in the receipt.
//
// A journey ID is this harness's stable name for a set of walks; receipts and
// critic fingerprints cite it. `capabilities` binds each journey to the
// SPEC.md capability heading it proves, and qa/walk refuses to run when that
// heading is gone. Add a walk here when its capability gains behavior.
export const capabilities={
 'US-001':'Earlier concept notebook',
 'US-002':'Earlier concept notebook',
 'US-003':'Grading',
 'US-004':'Retired generation records',
 'US-006':'Earlier concept notebook',
 'US-007':'Grading',
 'US-008':'Grading',
 'US-009':'Earlier concept notebook',
 'US-010':'Earlier concept notebook',
 'US-011':'Earlier concept notebook',
 'US-013':'Retired generation records',
 'US-014':'Owner tokens',
 'US-015':'Library',
 'US-016':'Scheduling',
 'US-017':'Grading',
 'US-018':'Open into study',
};
// The stored state of the earlier concept notebook each browser journey opens
// on, from tests/fixtures/earlier-notebook.json. A journey with no entry opens
// an unused space.
export const seeds={
 'US-001':'history-rich',
 'US-002':'question-cold-choice',
 'US-003':'result-jev-wrong',
 'US-004':'question-cold-choice',
 'US-006':'history-rich',
 'US-007':'result-jev-wrong',
 'US-008':'self-check-unsure',
 'US-009':'result-held',
 'US-010':'history-rich',
 'US-011':'empty-history',
};
const policy=(...names)=>async c=>c.policy(...names);
const map=async c=>{await c.page.goto(c.url('/map'));await c.page.getByRole('heading',{name:'A map of ideas.'}).waitFor();};
const shows=async(c,state)=>c.page.locator(`[data-state="${state}"]`).waitFor();
// Earlier practice offers nothing to answer, reveal, retry, dismiss or correct.
const readOnly=async c=>{
 assert.equal(await c.page.locator('main form,main button,main textarea,main [data-choice],main [data-poll],.choice').count(),0,'Earlier practice is read-only');
 assert.equal(await c.page.getByRole('button',{name:/Next question|Check my answer|I was right|Count as a miss|Retry|Show me/}).count(),0);
};
// Reading changed no stored row of the space on screen.
const unchanged=async(c,before)=>assert.deepEqual(await c.rows(),before,'A read wrote to the notebook');
// A Study answer through the starter's own native forms: start it, try from
// memory, answer. The earlier notebook beside it is left alone.
const studyAnswer=async(c,answer)=>{
 await c.page.goto(c.at('/?practice=nato'));
 if(await c.page.locator('form[action="/practice/nato/start"]').count())await c.page.locator('form[action="/practice/nato/start"] button').tap();
 else if(await c.page.locator('form[action="/practice/next"]').count())await c.page.locator('form[action="/practice/next"] button').tap();
 await c.page.locator('form[action="/practice/quiz"] button, #answer').first().waitFor();
 if(await c.page.locator('form[action="/practice/quiz"]').count())await c.page.locator('form[action="/practice/quiz"] button').tap();
 await c.page.locator('#answer').fill(answer);await c.page.locator('form[action="/practice/answer"] button.primary').tap();
 await c.page.locator('[data-state="exact-result"]').waitFor();
};
// The observed counts a Concept page must show, from the stored Events alone.
const observed=(app,concept)=>{
 const count={unaided:0,helped:0,missed:0};
 for(const event of app.events){
  const question=app.questions[event.question_id];
  if(event.concept_id!==concept||!question||question.archived||event.result.authority==='ungraded')continue;
  const correction=app.overrides.filter(o=>o.event_id===event.id).at(-1);
  const rating=event.assisted?0:correction?(correction.correct?3:1):event.result.rating;
  const outcome=correction?(correction.correct?'correct':'wrong'):event.result.outcome;
  if(rating===3&&!event.assisted)count.unaided++;
  else if(event.assisted||outcome==='revealed'||outcome.startsWith('warm_'))count.helped++;
  else if(rating===1)count.missed++;
 }
 return count;
};
export const specs={
 'US-001':[
  // The Archived notebook's retained bytes survive rows and complete archives
  // unchanged and never become learning; older archives stay readable.
  policy('retained_go_bytes_survive_rows_and_complete_archives_without_becoming_learning','ordinary_old_archives_and_rows_remain_compatible_without_custody','malformed_or_changed_retained_bytes_fail_before_restore'),
  async c=>{
   // Everything saved is reachable through secondary navigation and exported unchanged.
   const app=c.app,before=await c.rows();
   await c.page.locator('.account-menu>summary').tap();await c.page.getByRole('link',{name:'Earlier practice',exact:true}).tap();
   await c.page.locator('.understanding-nav').getByRole('link',{name:'Map',exact:true}).tap();await shows(c,'map');
   const goal=Object.values(app.goals).find(goal=>!goal.archived&&goal.concept_ids.some(id=>!app.concepts[id].archived));
   await c.page.locator(`a[href="/goals/${goal.id}"]`).first().tap();await shows(c,'goal');
   await c.page.getByRole('heading',{name:goal.title,exact:true}).waitFor();await c.page.locator('.saved-input').waitFor();
   const concept=app.concepts[goal.concept_ids.find(id=>!app.concepts[id].archived)];
   await c.page.locator(`main a[href="/concepts/${concept.id}"]`).first().tap();await shows(c,'concept');await c.page.locator('.reading').first().waitFor();
   const question=Object.values(app.questions).find(q=>q.concept_id===concept.id&&!q.archived);
   await c.page.locator('.question-reference summary').first().tap();
   assert.ok((await c.page.locator('.saved-questions').innerText()).includes(question.versions.at(-1).answer),'A saved question shows its answer');
   await c.page.locator('.account-menu>summary').tap();await c.page.getByRole('link',{name:'History',exact:true}).tap();await shows(c,'history');
   assert.equal(await c.page.locator('article.history-entry').count(),app.events.length,'Every recorded answer is in History');
   const exported=await c.page.request.get(c.at('/export'));assert.equal(exported.status(),200);
   const archive=await exported.json();assert.deepEqual(archive.app,app,'The export holds the notebook exactly as stored');assert.equal(archive.photos.length,1);
   for(const saved of Object.values(app.goals).filter(goal=>goal.photo))assert.equal((await c.page.request.get(c.at(`/photos/${saved.id}`))).status(),200);
   await unchanged(c,before);
   c.policy('every_state_round_trips_byte_for_byte','every_state_passes_full_validation','historical_schedule_policy_and_assistance_cannot_be_forged');
  }],
 'US-002':[
  // A question that was left unanswered is shown as unanswered, with what was presented.
  async c=>{await shows(c,'earlier-unanswered');await readOnly(c);await c.page.getByRole('heading',{name:'Not answered.',exact:true}).waitFor();assert.deepEqual(await c.page.locator('main ol li').allTextContents(),c.app.occurrence.presentation.choices);assert.equal(await c.page.locator('.explanation,.answer-pair').count(),0);await c.page.getByRole('heading',{level:1}).waitFor();},
  // A held result stays a held result: no Next.
  async c=>{await c.open('result-held');await shows(c,'earlier-result');await c.page.reload();await shows(c,'earlier-result');await c.page.locator('.answer-pair').waitFor();await readOnly(c);},
  // It blocks nothing: with the unanswered question held, the export and History answer.
  async c=>{const app=await c.open('question-cold'),before=await c.rows();await shows(c,'earlier-unanswered');assert.equal(await c.page.locator('details.overflow').count(),0);const exported=await c.page.request.get(c.at('/export'));assert.equal(exported.status(),200);assert.deepEqual((await exported.json()).app,app);assert.equal((await c.page.request.get(c.at('/history'))).status(),200);await unchanged(c,before);},
  async c=>{assert.deepEqual(await c.page.locator('.masthead nav a').allTextContents(),['Practice','Lessons','Library','History','Settings','Earlier practice','Changelog']);},
  // An answer saved without a Verdict says so, by the stored status of its check.
  async c=>{for(const [state,label] of [['checking-queued','Check recorded: queued, never sent.'],['checking-sent','Check recorded: sent, never confirmed.'],['self-check-failed','Check recorded: failed.'],['self-check-unknown','Check recorded: outcome unknown.']]){await c.open(state);await shows(c,'earlier-ungraded');await c.page.getByRole('heading',{name:'Your answer is saved, not graded.',exact:true}).waitFor();await c.page.getByText(label,{exact:true}).waitFor();await readOnly(c);assert.equal(await c.page.getByRole('link',{name:/Resume meaning checks|Check again/}).count(),0);}},
  policy('no_page_serves_an_earlier_question','a_refusal_and_a_resume_land_where_they_were_asked','current_generation_admission_refuses_every_old_entry_not_grading_or_external_authoring')
 ],
 'US-003':[
  // A Study answer that matches is Good.
  async c=>{await studyAnswer(c,'Alfa');await c.page.locator('[data-state="exact-result"] [data-verdict="Good"]').waitFor();c.policy('local_grading_accepts_only_authored_forms_and_choice_identity','an_exact_or_alias_match_is_good_at_once');},
  policy('short_answer_thresholds_and_risks_fail_closed','an_empty_answer_is_again_at_once_and_never_sent','malformed_unsure_injected_and_oversized_responses_leave_the_answer_ungraded'),
  policy('unlisted_valid_wording_is_accepted_by_jev_and_keeps_the_local_comparison','a_different_identity_is_rejected_and_a_risky_accept_is_not_a_grade'),
  // With paid judgment off, any other answer is Again and the result shows the expected content.
  async c=>{await studyAnswer(c,'not a code word');await c.page.locator('[data-state="exact-result"] [data-verdict="Again"]').waitFor();assert.match((await c.page.locator('[data-state="exact-result"] .nato-word').first().innerText()).trim(),/^[A-Z][A-Za-z-]+$/,'The result shows the expected code word');assert.equal(await c.page.getByRole('button',{name:'I was right',exact:true}).count(),0);},
  // A stored Jev Verdict keeps its Authority and is shown without being judged again.
  async c=>{const before=await c.rows();await c.page.goto(c.url('/'));await shows(c,'earlier-result');await c.page.getByRole('heading',{name:'Missed.',exact:true}).waitFor();await c.page.getByText('Meaning checked',{exact:true}).waitFor();assert.equal(c.app.occurrence.result.authority,'jev');await readOnly(c);await c.page.reload();await shows(c,'earlier-result');await unchanged(c,before);c.policy('the_result_and_history_pages_show_each_state_honestly');}
 ],
 'US-004':[
  policy('candidates_are_saved_before_critic_and_critic_retry_reuses_generation','normal_partial_output_is_honest_but_oversized_and_cyclic_output_fails'),
  policy('critic_requires_whole_applicable_battery_and_never_vetoes_teaching_score','critic_missing_extra_mixed_or_invalid_judgments_cannot_publish'),
  policy('allowance_is_shared_and_unknown_cost_is_never_free'),
  policy('explicit_critic_retry_sends_only_unfinished_batteries_and_preserves_attempts','fully_judged_rejection_is_reused_without_new_spend_or_score_fishing'),
  policy('skipped_critic_publishes_saved_candidates_without_a_second_reservation'),
  policy('export_readback_roundtrip_preserves_complete_schedule_and_evidence','paid_completion_receipts_are_immutable_including_unknown_cost_and_oversize')
 ],
 'US-006':[
  policy('evidence_quotes_must_match_saved_input_exactly_and_general_claims_have_no_quotes'),
  // A note that quotes supplied material keeps the exact evidence and its provenance;
  // general knowledge is labelled and never presented as quoted support.
  async c=>{
   const app=c.app,before=await c.rows(),latest=concept=>concept.notes.at(-1);
   const quoted=Object.values(app.concepts).find(concept=>latest(concept)?.quotes.length),general=Object.values(app.concepts).find(concept=>latest(concept)?.basis==='general');
   assert.ok(quoted&&general,'The fixture holds a quoting note and a general one');
   await c.page.goto(c.url(`/concepts/${quoted.id}`));await shows(c,'concept');
   const support=c.page.locator('.concept-note .provenance');await support.getByText('From your material',{exact:true}).waitFor();
   await support.locator('summary').tap();assert.deepEqual(await support.locator('blockquote').allTextContents(),latest(quoted).quotes);
   const material=app.goals[quoted.goal_id];for(const quote of latest(quoted).quotes)assert.ok((material.transcript??material.intent).includes(quote),'A quote is the saved material, word for word');
   await c.page.goto(c.url(`/concepts/${general.id}`));await shows(c,'concept');
   await c.page.locator('.concept-note .provenance').getByText('General knowledge',{exact:true}).waitFor();assert.equal(await c.page.locator('.concept-note blockquote').count(),0);
   await unchanged(c,before);
  },
  policy('evidence_quotes_must_match_saved_input_exactly_and_general_claims_have_no_quotes','author_and_model_text_is_escaped_in_every_context')
 ],
 'US-007':[
  // A Study Verdict offers no way to overturn it.
  async c=>{await studyAnswer(c,'Alpha');await c.page.locator('[data-state="exact-result"] [data-verdict="Again"]').waitFor();assert.equal(await c.page.getByRole('button',{name:/I was right|Count as a miss/}).count(),0);assert.equal(await c.page.locator('form[action="/practice/correct"]').count(),0);},
  // Neither does a Verdict saved in the earlier notebook.
  async c=>{await c.page.goto(c.url('/'));await shows(c,'earlier-result');assert.equal(await c.page.locator('form[action="/review/override"],form[action="/practice/correct"]').count(),0);await readOnly(c);c.policy('current_generation_admission_refuses_every_old_entry_not_grading_or_external_authoring','an_owner_correction_is_refused_on_an_answer_left_for_jev');},
  // A saved Correction stays readable and unchanged.
  async c=>{const app=await c.open('result-corrected'),before=await c.rows();await shows(c,'earlier-result');await c.page.getByText('Your Correction',{exact:true}).waitFor();assert.equal(app.overrides.length,1);await c.page.goto(c.url('/history'));await c.page.getByText('Grade corrected later',{exact:false}).first().waitFor();await unchanged(c,before);c.policy('corrections_are_unique_eligible_and_replay_the_original_attempt');}
 ],
 'US-008':[
  policy('local_grading_accepts_only_authored_forms_and_choice_identity'),
  policy('short_answer_thresholds_and_risks_fail_closed','a_refused_http_response_and_a_lost_send_leave_the_answer_ungraded','an_interrupted_send_becomes_unknown_once_and_is_never_sent_again'),
  // An answer no one could judge stays saved, shown and ungraded, with no self-grade.
  async c=>{const before=await c.rows();await shows(c,'earlier-ungraded');await c.page.getByRole('heading',{name:'Your answer is saved, not graded.',exact:true}).waitFor();await c.page.getByText('Check recorded: inconclusive.',{exact:true}).waitFor();assert.ok((await c.page.locator('.answer-pair').innerText()).includes(c.app.occurrence.answer));await readOnly(c);await c.page.goto(c.url('/history'));await shows(c,'history');await unchanged(c,before);}
 ],
 'US-009':[
  async c=>{await map(c);await c.page.locator('svg.constellation').waitFor({state:'attached'});await c.page.locator('.concept-list').waitFor();},
  // Prerequisite links both ways, and counts that are the stored Events and nothing more.
  async c=>{
   const app=c.app,before=await c.rows(),dependent=Object.values(app.concepts).find(concept=>concept.prerequisites.length),prerequisite=app.concepts[dependent.prerequisites[0]];
   for(const [concept,heading,other] of [[prerequisite,'Used by',dependent],[dependent,'Builds on',prerequisite]]){
    await c.page.goto(c.url(`/concepts/${concept.id}`));await shows(c,'concept');await c.page.getByRole('heading',{name:heading,exact:true}).waitFor();
    assert.equal(await c.page.locator('.connected').getByRole('link',{name:other.name,exact:true}).getAttribute('href'),`/concepts/${other.id}`);
    const count=observed(app,concept.id);
    assert.equal((await c.page.locator('.practice-evidence>p').first().innerText()).replace(/\s+/g,' ').trim(),`${count.unaided} recalled · ${count.helped} with help · ${count.missed} missed`);
   }
   assert.ok(app.events.length>0);await unchanged(c,before);
  },
  // Observed, never predicted: no estimate, label or brightness anywhere on it.
  async c=>{
   const app=c.app;await map(c);
   assert.equal(await c.page.locator('svg.constellation [class*="brightness-"]').count(),0,'No star is brightened by a prediction');
   for(const path of ['/map',...Object.keys(app.concepts).map(id=>`/concepts/${id}`)]){await c.page.goto(c.at(path));assert.doesNotMatch(await c.page.locator('main').innerText(),/Estimated recall|recall estimate|Solid|Fading|next practice/,path);}
   c.policy('observations_distinguish_exposure_help_and_cold_recall_without_mutations','no_earlier_page_offers_a_write_or_a_prediction');
  }
 ],
 'US-010':[
  // Nothing is rewritten or deleted. No page offers a write: the Map, every
  // Goal, Concept and saved question, Earlier practice and Settings.
  async c=>{
   const app=c.app,before=await c.rows();
   const pages=['/notebook','/map',...Object.keys(app.goals).map(id=>`/goals/${id}`),...Object.keys(app.concepts).map(id=>`/concepts/${id}`),...Object.keys(app.questions).map(id=>`/questions/${id}/edit`)];
   for(const path of pages){await c.page.goto(c.at(path));assert.equal(await c.page.locator('main form,main button,main textarea,main input').count(),0,`${path} offers a write`);}
   const question=Object.values(app.questions).find(q=>!q.archived&&q.versions.at(-1).choices.length);
   await c.page.goto(c.at(`/questions/${question.id}/edit`));await shows(c,'earlier-question');
   await c.page.getByRole('heading',{level:1,name:question.versions.at(-1).prompt,exact:true}).waitFor();
   assert.deepEqual(await c.page.locator('main ol li').allTextContents(),question.versions.at(-1).choices);
   assert.equal((await c.page.locator('.answer-pair dd').first().innerText()).trim(),question.versions.at(-1).answer);
   await c.page.goto(c.at('/settings'));await shows(c,'settings');
   assert.equal(await c.page.locator('input[name="pace"],form[action="/settings"]').count(),0,'Settings sets no earlier pace');
   assert.equal(await c.page.getByText('New items per day').count(),0);
   await c.page.locator('form[action="/settings/backup"]').waitFor({state:'attached'});
   await unchanged(c,before);
  },
  // Every retired write answers 410 with the same message, before anything is
  // written, a replay of a committed one included; the rows are identical.
  async c=>{
   const app=c.app,origin=new URL(c.at('/')).origin,csrf=JSON.parse((await c.rows()).meta).csrf;
   // An agent's Owner token, for the API route; minting it is the one write here.
   const minted=await c.page.request.post(c.at('/tokens/create'),{form:{csrf,operation_id:'walk-us010-agent-token',name:'Synthetic US-010 agent'},headers:{Origin:origin},maxRedirects:0});
   assert.equal(minted.status(),200);const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())[1].trim();
   const before=await c.rows();
   const writes=[
    ...Object.keys(app.goals).flatMap(id=>['pause','focus','archive'].map(action=>`/goals/${id}/${action}`)),
    ...Object.keys(app.concepts).flatMap(id=>['archive','feedback'].map(action=>`/concepts/${id}/${action}`)),
    ...Object.keys(app.questions).flatMap(id=>['edit','archive','dispute'].map(action=>`/questions/${id}/${action}`)),
    '/settings','/__fixture',
   ];
   const committed=Object.entries(app.operations).filter(([,receipt])=>writes.includes(receipt.path));
   assert.ok(committed.length>=8,'The fixture holds committed operations on the retired routes');
   for(const [path,operation_id] of [...writes.map(path=>[path,'walk-retired-'+path.replace(/[^a-z0-9]/gi,'-').slice(-90)]),...committed.map(([id,receipt])=>[receipt.path,id])]){
    const response=await c.page.request.post(c.at(path),{form:{csrf,operation_id,pace:'light',text:'synthetic'},headers:{Origin:origin},maxRedirects:0});
    assert.equal(response.status(),410,`POST ${path}`);assert.match(await response.text(),/The earlier concept notebook is kept as history\./,path);
   }
   const api=await c.page.request.post(c.at('/api/v1/settings'),{data:{operation_id:'walk-retired-api-settings',pace:'light'},headers:{Authorization:'Bearer '+secret},maxRedirects:0});
   assert.equal(api.status(),410,'POST /api/v1/settings');assert.match((await api.json()).error,/kept as history/);
   await unchanged(c,before);
   c.policy('every_earlier_write_is_refused_and_shown_where_it_was_asked','current_generation_admission_refuses_every_old_entry_not_grading_or_external_authoring');
  },
  // Stored flags are shown as stored and exported unchanged: focus, pause and
  // the pace preference, which nothing sets any more.
  async c=>{
   const app=c.app,before=await c.rows();
   const shown=Object.values(app.goals).filter(goal=>!goal.archived);
   assert.ok(shown.some(goal=>goal.focused)&&shown.some(goal=>goal.paused),'The fixture holds a focused and a paused Goal');
   await map(c);
   for(const goal of shown){
    const meta=(await c.page.locator('.goal-map').filter({has:c.page.locator(`h2 a[href="/goals/${goal.id}"]`)}).locator('.goal-meta').innerText());
    assert.equal(meta.includes('In focus'),goal.focused,goal.id);assert.equal(meta.includes('Paused'),goal.paused,goal.id);
   }
   const exported=await(await c.page.request.get(c.at('/export'))).json();
   assert.deepEqual(exported.app.preferences,app.preferences);assert.equal(exported.app.preferences.pace,'light');
   for(const goal of Object.values(app.goals))assert.deepEqual([exported.app.goals[goal.id].focused,exported.app.goals[goal.id].paused],[goal.focused,goal.paused]);
   await unchanged(c,before);
  }
 ],
 'US-011':[
  // Not served or scheduled: with a question due and nothing held, reading every
  // page selects nothing and writes nothing.
  async c=>{
   const app=c.app,before=await c.rows();assert.equal(app.occurrence,null);assert.ok(Object.values(app.questions).some(question=>!question.archived),'A question is saved and due');
   await shows(c,'earlier-empty');await readOnly(c);
   for(const path of ['/map','/history','/settings',...Object.keys(app.goals).map(id=>`/goals/${id}`),...Object.keys(app.concepts).map(id=>`/concepts/${id}`),'/notebook']){await c.page.goto(c.at(path));assert.equal(await c.page.locator('main form[action^="/review/"],main form[action$="/practice"]').count(),0,path);}
   assert.equal((await c.page.request.get(c.at('/export'))).status(),200);
   await shows(c,'earlier-empty');await unchanged(c,before);
  },
  // Not graded: a check saved as queued is still queued, and nothing was sent or finished.
  async c=>{await c.open('checking-queued');const before=await c.rows();await shows(c,'earlier-ungraded');await c.page.getByText('Check recorded: queued, never sent.',{exact:true}).waitFor();await c.page.waitForTimeout(1500);await c.page.reload();await shows(c,'earlier-ungraded');assert.equal(await c.page.locator('[data-poll]').count(),0);await unchanged(c,before);},
  // Study never draws on it.
  async c=>{
   const earlier=rows=>Object.fromEntries(Object.entries(rows).filter(([key])=>/^(goals|concepts|questions|events|overrides|resets|feedback|assessments|jobs)\//.test(key)));
   const before=await c.rows(),held=JSON.parse(before.meta).occurrence;await studyAnswer(c,'Alfa');const after=await c.rows();
   assert.deepEqual(earlier(after),earlier(before),'Study changed the earlier notebook');assert.deepEqual(JSON.parse(after.meta).occurrence,held);
   const versions=Object.keys(after).filter(key=>key.startsWith('library_versions/')).map(key=>key.slice('library_versions/'.length));
   const attempts=Object.keys(after).filter(key=>key.startsWith('practice_attempts/')).map(key=>JSON.parse(after[key]));
   assert.equal(attempts.length,1);assert.ok(versions.includes(attempts[0].version_id),'The Prompt Study served is of a Library Version');
   c.policy('nothing_the_runtime_runs_completes_a_frozen_check','a_study_check_is_claimed_and_recovered_beside_a_frozen_one','study_beside_the_earlier_notebook_leaves_it_as_stored','a_held_earlier_question_blocks_and_masks_nothing');
  }
 ],
 'US-013':[
  async c=>{await c.page.goto(c.url('/create'));await c.page.getByText('Native content generation is retired.',{exact:true}).waitFor();assert.equal(await c.page.locator('form[action="/create"],input[type="file"],input[name="mode"]').count(),0);c.policy('capture_word_phrase_dictated_and_url_stay_private_and_exact');},
  async c=>{await c.page.goto(c.url('/create?text=https%3A%2F%2Fprivate.invalid%2Fnotes'));const draft=c.page.getByLabel('Your unsent material',{exact:true});assert.equal(await draft.inputValue(),'https://private.invalid/notes');assert.equal(await draft.getAttribute('readonly'),'');assert.equal(await c.page.locator('form[action="/create"],input[type="file"]').count(),0);c.policy('failures_do_not_partially_capture_and_csrf_is_required','capture_word_phrase_dictated_and_url_stay_private_and_exact');}
 ],
 'US-014':[
  async c=>{
    await c.owner.criteria[0]();
    await c.page.goto(c.url('/settings'));
    await c.page.locator('#owner-tokens > summary').click();
    await c.page.getByRole('textbox',{name:'Name',exact:true}).fill('browser agent');
    await c.page.getByRole('button',{name:'Create Owner token',exact:true}).click();
    await c.page.getByRole('heading',{name:'Save your Owner token'}).waitFor();
    // Never retain a secret in screenshots or artifacts.
    await c.page.getByRole('link',{name:'Return to Owner tokens'}).click();
    await c.page.locator('#owner-tokens > summary').click();
    await c.page.getByRole('heading',{name:'browser agent',exact:true}).waitFor();
  },
  ...Array.from({length:7},(_,i)=>async c=>c.owner.criteria[i+1]())
 ],
 'US-015':Array.from({length:8},(_,i)=>async c=>{
   if(i===6)c.policy('supersede_retains_original_and_study_reference_refuses_deletion');
   await c.library.criteria[i]();
 }),
 'US-016':Array.from({length:11},(_,i)=>async c=>c.practice.studyCriteria[i]()),
 'US-017':Array.from({length:10},(_,i)=>async c=>c.practice.graderCriteria[i]()),
 'US-018':[...Array.from({length:5},(_,i)=>async c=>{
   c.policy('phone_stream_uses_shared_prompt_verdict_diff_and_correction_receipts');
   assert.equal(c.phone.length,10);
   for(const row of c.phone){assert.equal(row.status,'pass',row.reason);assert.ok(row.criteria.includes(i+1),`US-018.${i+1}: missing ${row.width}px ${row.scheme} observation`);}
 }),async c=>{
   assert.equal(c.library.length,10);
   for(const row of c.library){assert.equal(row.status,'pass',row.reason);assert.ok(row.criteria.includes(6),`US-018.6: missing ${row.width}px ${row.scheme} entry observation`);}
   assert.equal(c.libraryRecovery.length,6);
   for(const row of c.libraryRecovery)assert.equal(row.status,'pass',`${row.name}: ${row.reason}`);
 }],
};
