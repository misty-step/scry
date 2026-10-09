import assert from 'node:assert/strict';

// Current numbered criteria, scoped to synthetic browser/policy mechanics.
// Live provider usefulness, production, real phone and independent hosted
// recovery acceptance remain explicitly unverified in the receipt.
const policy=(...names)=>async c=>c.policy(...names);
const choice=async(c,index=0)=>{await c.page.locator('.choice').nth(index).click();await c.page.locator('[data-state="result"]').waitFor();};
const map=async c=>{await c.page.goto(c.url('/map'));await c.page.getByRole('heading',{name:'A map of ideas.'}).waitFor();};
const recall=async c=>{await choice(c);await c.page.getByRole('button',{name:'Next question'}).click();await c.page.locator('#answer').waitFor();};
const concept=async c=>{await choice(c);await c.page.locator('.reference-link').click();await c.page.locator('.concept-page').waitFor();};
export const specs={
 'US-001':[c=>c.retainedGoMigration(),async c=>{await concept(c);await c.page.locator('.reading').first().waitFor();c.policy('content_edits_leave_historical_presentations_immutable');}],
 'US-002':[
  async c=>{assert.equal(await c.page.locator('.choice').count(),3);assert.equal(await c.page.locator('.explanation,.answer-pair').count(),0);await c.page.getByRole('heading',{level:1}).waitFor();},
  async c=>{await choice(c);await c.page.reload();await c.page.locator('[data-state="result"]').waitFor();await c.page.locator('.answer-pair').waitFor();assert.equal(await c.page.getByRole('button',{name:'Next question'}).count(),1);},
  async c=>{await c.page.locator('details.overflow>summary').click();await c.page.getByRole('button',{name:'Count as a miss'}).waitFor();},
  async c=>{assert.deepEqual(await c.page.locator('.masthead nav a').allTextContents(),['Create','Map']);},
  policy('reveal_is_held_and_next_is_deliberate','competing_occurrences_and_archive_cannot_publish_stale_answer')
 ],
 'US-003':[
  async c=>{await choice(c);c.policy('local_grading_accepts_only_authored_forms_and_choice_identity');},
  policy('short_answer_thresholds_and_risks_fail_closed','assessment_send_once_timeout_unknown_reservation_and_explicit_retry'),
  policy('required_idea_policy_and_shadow_classes_keep_honest_authority'),
  async c=>{await c.page.getByRole('button',{name:'Next question'}).click();await c.page.locator('#answer').fill('a plausible synthetic nonexact answer');await c.page.getByRole('button',{name:'Check my answer'}).click();await c.page.locator('[data-state="self-check"]').waitFor();assert.match(await c.page.locator('.answer-pair').innerText(),/plausible synthetic/);await c.page.getByRole('button',{name:'I was right',exact:true}).click();await c.page.locator('[data-state="result"]').waitFor();},
  async c=>{assert.match(await c.page.locator('.feedback').innerText(),/You checked your answer/);c.policy('missing_assessor_keeps_answer_and_learner_authority','exact_answer_event_schedule_and_receipt_are_one_transition');}
 ],
 'US-004':[
  policy('candidates_are_saved_before_critic_and_critic_retry_reuses_generation','normal_partial_output_is_honest_but_oversized_and_cyclic_output_fails'),
  policy('critic_requires_whole_applicable_battery_and_never_vetoes_teaching_score','critic_missing_extra_mixed_or_invalid_judgments_cannot_publish'),
  policy('allowance_is_shared_and_unknown_cost_is_never_free','assessment_send_once_timeout_unknown_reservation_and_explicit_retry'),
  policy('explicit_critic_retry_sends_only_unfinished_batteries_and_preserves_attempts','fully_judged_rejection_is_reused_without_new_spend_or_score_fishing'),
  policy('skipped_critic_publishes_saved_candidates_without_a_second_reservation'),
  policy('export_readback_roundtrip_preserves_complete_schedule_and_evidence','paid_completion_receipts_are_immutable_including_unknown_cost_and_oversize')
 ],
 'US-006':[
  policy('evidence_quotes_must_match_saved_input_exactly_and_general_claims_have_no_quotes'),
  async c=>{await concept(c);await c.page.goto(c.url('/'));await c.page.locator('[data-state="result"]').waitFor();},
  policy('evidence_quotes_must_match_saved_input_exactly_and_general_claims_have_no_quotes','author_and_model_text_is_escaped_in_every_context')
 ],
 'US-007':[
  async c=>{await choice(c,1);await c.page.getByRole('button',{name:'I was right',exact:true}).waitFor();},
  async c=>{await c.page.getByRole('button',{name:'I was right',exact:true}).click();await c.page.locator('[data-state="result"]').waitFor();await c.page.getByText('You corrected this grade',{exact:false}).waitFor();},
  policy('semantic_verdict_fenced_and_grade_override_keeps_original')
 ],
 'US-008':[
  policy('local_grading_accepts_only_authored_forms_and_choice_identity'),
  policy('short_answer_thresholds_and_risks_fail_closed'),
  async c=>{await recall(c);await c.page.locator('#answer').fill('nonexact synthetic answer');await c.page.getByRole('button',{name:'Check my answer'}).click();await c.page.locator('[data-state="self-check"]').waitFor();assert.match(await c.page.locator('.answer-pair').innerText(),/nonexact synthetic answer/);}
 ],
 'US-009':[
  async c=>{await map(c);await c.page.locator('svg.constellation').waitFor({state:'attached'});await c.page.locator('.concept-list').waitFor();},
  policy('observations_distinguish_exposure_help_and_cold_recall_without_mutations','solid_and_fading_are_recall_estimates_and_latest_help_prevents_solid')
 ],
 'US-010':[
  async c=>{await map(c);await c.page.getByRole('button',{name:'Focus here',exact:true}).click();await c.page.getByRole('button',{name:'Remove focus',exact:true}).waitFor();},
  async c=>{await c.page.getByRole('button',{name:'Pause',exact:true}).click();await c.page.getByRole('button',{name:'Resume',exact:true}).click();await c.page.getByRole('button',{name:'Pause',exact:true}).waitFor();},
  async c=>{await c.page.goto(c.url('/settings'));await c.page.locator('input[name="pace"][value="light"]').check();await c.page.getByRole('button',{name:'Save my rhythm'}).click();assert.equal(await c.page.locator('input[name="pace"][value="light"]').isChecked(),true);}
 ],
 'US-011':[
  policy('normal_partial_output_is_honest_but_oversized_and_cyclic_output_fails'),
  policy('observations_distinguish_exposure_help_and_cold_recall_without_mutations','reveal_is_held_and_next_is_deliberate')
 ],
 'US-013':[
  async c=>{await c.page.goto(c.url('/create'));assert.equal(await c.page.locator('input[name="mode"]').count(),0);await c.page.locator('textarea[name="intent"]').fill('Learn cache validation using practical examples.');await c.page.getByRole('button',{name:'Make it understandable'}).click();await c.page.locator('[data-state="goal"]').waitFor();await c.page.locator('.saved-input>summary').click();assert.match(await c.page.locator('.saved-input').innerText(),/Learn cache validation/);c.policy('capture_word_phrase_dictated_and_url_stay_private_and_exact');},
  async c=>{await c.page.goto(c.url('/create?text=https%3A%2F%2Fprivate.invalid%2Fnotes'));assert.equal(await c.page.locator('textarea[name="intent"]').inputValue(),'https://private.invalid/notes');c.policy('failures_do_not_partially_capture_and_csrf_is_required','capture_word_phrase_dictated_and_url_stay_private_and_exact');}
 ]
};
