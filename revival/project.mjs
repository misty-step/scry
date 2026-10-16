// One-way ADR 006 projection. Input is read only by the validated CF SQLite reader.
// No provider calls, historical job execution, or learner-facing compatibility API.
const encoder = new TextEncoder();
// Only require constructs this type, with source-literal codes, never a caught
// exception's message. All parser/SQL/storage errors retain the generic response.
export class ProjectionRefusal extends Error {}
export const algorithm = 'go-fsrs/v4.0.0;defaults-v1;retention=.9;fuzz=false;steps=1m,10m;relearn=10m;grading=exact-v1';
export function project(tables, bytes, checksum, created) {return runProjection(tables, bytes, checksum, created);}
export function diagnose(tables, bytes, checksum, created) {
  const refusals = new Map();
  runProjection(tables, bytes, checksum, created, refusals);
  return Object.fromEntries([...refusals].sort(([a], [b]) => a.localeCompare(b)));
}
function runProjection(tables, originalBytes, originalHash, created, refusals = null) {
  let incomplete = false, seen = new Set();
  // Conversion still throws at its first refusal. Diagnosis evaluates the same
  // guards without throwing, counting a class once per visited record. A failed
  // dependent expression aborts only that record, but invalidates the ENTIRE
  // report; no partial traversal is ever returned as a complete diagnostic.
  const require = (condition, reason = 'invariant') => {
    if (!condition) {
      if (!refusals) throw new ProjectionRefusal(reason);
      if (!seen.has(reason)) refusals.set(reason, (refusals.get(reason) || 0) + 1);
      seen.add(reason);
    }
    return Boolean(condition);
  };
  const visit = (records, consume) => {
    const values = [];
    for (const record of records) {
      const parent = seen;
      seen = new Set();
      try {values.push(consume(record));} catch (error) {
        if (!refusals) throw error;
        incomplete = true;
      } finally {seen = parent;}
    }
    return values;
  };
  const json = text => JSON.parse(text);
  const own = (object, key) => Object.hasOwn(object, key);
  const by = (records, field, value) => records.filter(record => record[field] === value);
  const unique = records => {require(records.length === 1, 'cardinality'); return records[0];};
  const id = (kind, original) => {
    require(typeof original === 'string' && /^[A-Za-z0-9_-]{1,80}$/.test(original), 'identifier');
    return 'legacy-import-' + kind + '-' + original;
  };
  const timestamp = text => {
    require(typeof text === 'string' && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,3}0*)?(?:Z|[+-]\d\d:\d\d)$/.test(text), 'timestamp-precision');
    const ms = Date.parse(text);
    require(Number.isSafeInteger(ms), 'timestamp-range');
    return ms;
  };
  function card(text) {
    const c = json(text);
    require(Object.keys(c).sort().join(',') === 'Difficulty,Due,Lapses,LastReview,RemainingSteps,Reps,ScheduledDays,Stability,State', 'card-shape');
    require(Number.isInteger(c.State) && c.State >= 0 && c.State <= 3, 'card-state');
    for (const field of ['ScheduledDays', 'Reps', 'Lapses']) require(Number.isSafeInteger(c[field]) && c[field] >= 0, 'card-count');
    require(Number.isInteger(c.RemainingSteps) && c.RemainingSteps >= 0 && c.RemainingSteps <= 2, 'card-steps');
    require(Number.isFinite(c.Stability) && Number.isFinite(c.Difficulty), 'card-real');
    return {due_ms: timestamp(c.Due), stability: c.Stability, difficulty: c.Difficulty,
      scheduled_days: c.ScheduledDays, reps: c.Reps, lapses: c.Lapses,
      state: ['new', 'learning', 'review', 'relearning'][c.State],
      last_review_ms: c.LastReview === '0001-01-01T00:00:00Z' ? null : timestamp(c.LastReview),
      remaining_steps: c.RemainingSteps};
  }
  function basis(value, evidence) {
    require(value === 'topic' || value === 'source', 'content-basis');
    const quotes = Array.isArray(evidence) ? evidence : evidence ? [evidence] : [];
    require(quotes.every(quote => typeof quote === 'string'), 'content-quotes');
    require(value !== 'topic' || quotes.length === 0, 'topic-quotes');
    return {basis: value === 'topic' ? 'general' : 'source', quotes};
  }
  function content(record) {
    const q = json(record.content);
    require(q.kind === 'recall' || q.kind === 'choice', 'content-kind');
    require(!q.grading || q.grading === 'exact' || q.grading === 'semantic', 'content-grading');
    return {version: record.version, prompt: q.prompt,
      kind: q.grading === 'semantic' ? 'explain' : q.kind, answer: q.answer,
      variants: q.variants || [], choices: q.choices || [], explanation: q.explanation,
      ...basis(q.basis, q.evidence), required_ideas: (q.rubric?.required || []).map(idea => idea.text),
      contradictions: (q.rubric?.contradictions || []).map(claim => claim.text),
      model: record.model, created_ms: record.created_at};
  }
  function preparation(original, goal, ready) {
    return {id: id('job', original.id), goal_id: goal.id, question_id: null, question_version: null,
      source_revision: original.source_revision, kind: 'create',
      status: original.status === 'complete' ? 'ready' : original.status === 'partial' ? 'partial' :
        original.status === 'failed' ? 'failed' : original.status === 'canceled' ? 'superseded' : 'paused',
      created_ms: original.created_at, lease: null, started_ms: null, attempt: Math.max(1, original.attempts),
      error: ready ? null : 'legacy-import: historical work is paused; original receipts remain in the Go archive.',
      raw_response: null, model: original.model || 'legacy-import', feedback_ids: [], rejected: [],
      candidate_json: null, critic_json: null};
  }
  function put(map, record) {require(!own(map, record.id)); map[record.id] = record; return record;}
  const base64 = bytes => {
    let text = '';
    for (let start = 0; start < bytes.length; start += 8192) text += String.fromCharCode(...bytes.subarray(start, start + 8192));
    return btoa(text);
  };

  require(/^[a-f0-9]{64}$/.test(originalHash) && Number.isSafeInteger(created) && created > 0);
  const t = name => {require(Array.isArray(tables[name])); return tables[name];};
  // Unknown active constructs are never silently converted to invented learning.
  // They stay byte-preserved, but this version refuses to prepare a cutover archive.
  visit([...t('corrections'), ...t('semantic_assessments')], () => require(false, 'unsupported-assessments-or-corrections'));
  visit(t('source_documents'), () => require(false, 'unsupported-web-documents'));
  let preferences;
  visit([t('preferences')], rows => {preferences = {pace: unique(rows).pace};});
  const app = {schema: 1, revision: 0, restored_paused: true, csrf: 'legacy-import-' + originalHash,
    goals: {}, concepts: {}, questions: {}, occurrence: null, events: [], overrides: [], resets: [],
    operations: {}, jobs: {}, assessments: {}, spend: [], feedback: [],
    preferences,
    backup: {completed_ms: null, key: null, sha256: null, error: null}, revival_go: {}, revival_schedule_origins: {}};
  const goalForSource = new Map();
  let createdConcepts = 0, createdNotes = 0;
  visit(t('sources'), source => {
    const old = unique(by(t('goals'), 'source_id', source.id));
    require(['active', 'paused', 'archived'].includes(old.status), 'goal-status');
    const quizzes = by(t('quizzes'), 'source_id', source.id);
    const jobs = by(t('jobs'), 'source_id', source.id).sort((a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id));
    require(jobs.length > 0, 'source-without-job');
    const ready = quizzes.length > 0;
    const goal = put(app.goals, {id: id('goal', old.id), title: old.title, intent: source.text,
      created_ms: old.created_at, revision: source.revision, status: ready ? 'ready' : 'paused',
      paused: old.status === 'paused', focused: old.focus === 1,
      archived: old.status === 'archived' || source.archived === 1,
      concept_ids: [], job_id: id('job', jobs.at(-1).id), photo: null, transcript: null});
    goalForSource.set(source.id, goal);
    visit(jobs, job => put(app.jobs, preparation(job, goal, ready)));
  });
  require(Object.keys(app.goals).length === t('goals').length);
  const photos = [];
  visit(t('capture_images'), image => {
    const goal = goalForSource.get(image.source_id);
    require(goal && image.bytes instanceof Uint8Array && image.bytes.length > 0);
    // Hashes are supplied by the same CF reader, computed over all blob bytes.
    require(/^[a-f0-9]{64}$/.test(image.sha256));
    const photo = {key: 'photos/legacy-import/' + image.source_id, mime: image.mime,
      size: image.bytes.length, sha256: image.sha256};
    goal.photo = photo;
    photos.push({photo, bytes: base64(image.bytes)});
  });
  // Preserve current generated ideas/notes per owning source. Foundation-origin
  // concepts remain historical; they are never revived as new introductions.
  visit(t('concepts').filter(c => c.origin !== 'foundation'), original => {
    require(original.status === 'active' || original.status === 'archived', 'concept-status');
    const goal = goalForSource.get(original.source_id);
    require(goal, 'concept-goal');
    const notes = by(t('notes'), 'concept_id', original.id).sort((a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id));
    require(notes.length > 0, 'concept-without-note');
    const observations = by(t('evidence'), 'concept_id', original.id);
    const intros = observations.filter(e => e.kind === 'read' || e.kind === 'know');
    const assessed = by(t('concept_quizzes'), 'concept_id', original.id).map(link => link.quiz_id);
    const exposures = [...intros, ...t('assistance_exposures').filter(e => assessed.includes(e.quiz_id))];
    const concept = put(app.concepts, {id: id('concept', original.id), goal_id: goal.id,
      name: original.name, summary: original.description,
      notes: visit(notes, note => ({text: note.body, ...basis(note.basis, json(note.evidence)),
        created_ms: note.created_at, model: note.model})), prerequisites: [],
      introduced_ms: intros.length ? Math.min(...intros.map(e => e.created_at)) : null,
      exposure_ms: exposures.length ? Math.max(...exposures.map(e => e.created_at)) : null,
      already_known: intros.some(e => e.kind === 'know'), archived: original.status === 'archived'});
    goal.concept_ids.push(concept.id);
  });
  visit(t('concept_relations').filter(r => r.retired_at === 0 && r.origin !== 'foundation' && r.kind === 'requires'), relation => {
    const from = app.concepts[id('concept', relation.from_id)], to = app.concepts[id('concept', relation.to_id)];
    require(from && to && from.goal_id === to.goal_id);
    from.prerequisites.push(to.id);
  });
  visit(t('quizzes'), original => {
    const goal = goalForSource.get(original.source_id);
    require(goal);
    const versions = by(t('quiz_versions'), 'quiz_id', original.id).sort((a, b) => a.version - b.version);
    require(versions.length === original.version && versions.every((v, i) => v.version === i + 1), 'content-version-chain');
    const schedule = unique(by(t('schedules'), 'quiz_id', original.id));
    require(schedule.algorithm === algorithm, 'scheduler-algorithm');
    const fullCard = card(schedule.card);
    require(fullCard.due_ms === schedule.due_at, 'scheduler-due');
    const active = by(t('concept_quizzes'), 'quiz_id', original.id).filter(link => link.role === 'assesses' && own(app.concepts, id('concept', link.concept_id)));
    require(active.length <= 1, 'ambiguous-primary-concept');
    let concept = active.length ? app.concepts[id('concept', active[0].concept_id)] : null;
    if (!concept) {
      const current = json(versions.at(-1).content);
      concept = put(app.concepts, {id: id('quiz-concept', original.id), goal_id: goal.id,
        name: 'Legacy import — quiz ' + original.id,
        summary: 'Legacy import: reference assembled mechanically from the original quiz, not new learning evidence.',
        notes: [{text: 'Legacy import\n\n' + current.prompt + '\n\n' + current.answer + '\n\n' + current.explanation,
          basis: 'general', quotes: [], created_ms: created, model: 'legacy-import'}],
        prerequisites: [], introduced_ms: null, exposure_ms: null, already_known: false, archived: false});
      goal.concept_ids.push(concept.id);
      createdConcepts++;
      createdNotes++;
    }
    require(concept.goal_id === goal.id);
    put(app.questions, {id: id('quiz', original.id), goal_id: goal.id, concept_id: concept.id,
      versions: visit(versions, content), card: fullCard, schedule_version: schedule.version,
      archived: false, draft: null, draft_for_version: null, position: original.origin_index});
    app.revival_schedule_origins[id('quiz', original.id)] = 1;
  });
  require(Object.keys(app.questions).length === t('schedules').length);
  const presentations = new Map(t('presentations').map(p => [p.id, p]));
  visit([...t('review_events')].sort((a, b) => a.reviewed_at - b.reviewed_at || a.id.localeCompare(b.id)), old => {
    const observation = old.rating === 0;
    const ungraded = ['selfcheck', 'warm_selfcheck'].includes(old.outcome);
    require([0, 1, 3].includes(old.rating) && old.schedule_version_after === old.schedule_version_before + (observation ? 0 : 1), 'event-transition');
    require(!observation || (old.assisted === 1 && ['warm_correct', 'warm_wrong'].includes(old.outcome)) ||
      (ungraded && old.grading === 'exact-v1' && (old.outcome !== 'warm_selfcheck' || old.assisted === 1)), 'event-observation');
    require(!ungraded || observation, 'event-observation');
    const presented = presentations.get(old.presentation_id);
    require(presented);
    const q = app.questions[id('quiz', presented.quiz_id)];
    require(q);
    const presentation = q.versions[presented.content_version - 1];
    const snapshot = json(old.snapshot);
    // Collection mode must still visit nested guards after a version refusal;
    // conversion continues to throw at that same first refusal.
    require(snapshot.version === presentation.version, 'event-content');
    require(JSON.stringify(content({content: old.snapshot,
      version: presentation.version, model: presentation.model, created_at: presentation.created_ms})) === JSON.stringify(presentation), 'event-content');
    const authority = ungraded ? 'ungraded' : old.outcome === 'revealed' ? 'reveal' : old.grading === 'exact-v1' ? 'exact' :
      old.grading === 'learner-v1' ? 'learner' : ['short-v1', 'semantic-v1'].includes(old.grading) ? 'jev' : null;
    require(authority, 'event-authority');
    const before = card(old.schedule_before), after = card(old.schedule_after);
    require(!observation || JSON.stringify(before) === JSON.stringify(after), 'event-observation-card');
    // Go leaves the presentation's due_at sentinel at zero when no schedule
    // was written. Rust's result names the unchanged real card's due date.
    require(after.due_ms === old.due_at || (observation && old.due_at === 0), 'event-due');
    const event = {id: id('event', old.id), occurrence_id: id('presentation', old.presentation_id),
      question_id: q.id, concept_id: q.concept_id, presentation, answer: old.answer,
      assisted: old.assisted === 1, before_card: before, after_card: after,
      schedule_version: old.schedule_version_after, algorithm: old.algorithm,
      result: {event_id: id('event', old.id), outcome: old.outcome, authority, rating: old.rating,
        reviewed_ms: old.reviewed_at, due_ms: after.due_ms, corrected: false}};
    app.events.push(event);
  });
  visit(t('grade_overrides'), old => {
    require(app.events.some(e => e.id === id('event', old.review_id)));
    app.overrides.push({id: id('override', old.id), event_id: id('event', old.review_id),
      correct: old.direction === 'correct', before_card: card(old.schedule_before),
      after_card: card(old.schedule_after), created_ms: old.created_at});
  });
  visit([t('review_session')], rows => {
  const current = unique(rows).current_id;
  if (current) {
    const old = presentations.get(current);
    require(old);
    const q = app.questions[id('quiz', old.quiz_id)];
    require(q);
    const event = old.review_id ? app.events.find(e => e.id === id('event', old.review_id)) : null;
    const ungraded = event?.result.authority === 'ungraded';
    require(ungraded || (old.graded === 1) === Boolean(event), 'current-event');
    const result = event && !ungraded ? structuredClone(event.result) : null;
    const override = event && app.overrides.find(o => o.event_id === event.id);
    if (override) Object.assign(result, {outcome: override.correct ? 'correct' : 'wrong', authority: 'learner',
      corrected: true, rating: override.correct ? 3 : 1, due_ms: override.after_card.due_ms});
    app.occurrence = {id: id('presentation', current), question_id: q.id, concept_id: q.concept_id,
      presentation: q.versions[old.content_version - 1], phase: ungraded ? 'self-check' : event ? 'result' : 'question',
      assisted: old.assisted === 1, before_card: event ? event.before_card : q.card,
      schedule_version: old.schedule_version, created_ms: old.created_at,
      answer: event ? old.answer : null, assessment_id: null, result};
  }
  });
  // Keep receipt IDs/hashes to reject reuse. Original kind/result JSON are in
  // verbatim custody, not executable Go API responses on the new runtime.
  visit(t('operations'), old => {
    require(!own(app.operations, old.id));
    app.operations[old.id] = {payload_hash: old.payload_hash,
      result: {redirect: '/history', entity_id: null, message: 'legacy-import: original receipt retained in the Go archive.'}};
  });
  const paid = (old, spendId, workId, when, model, pending) => {
    require(Number.isSafeInteger(when) && when > 0);
    require(Number.isSafeInteger(old.reserved_micros) && old.reserved_micros >= 0);
    require(old.cost_micros === null || (Number.isSafeInteger(old.cost_micros) && old.cost_micros >= 0));
    require(old.cost_micros !== null || old.reserved_micros === 500000, 'unknown-reservation');
    require(typeof model === 'string' && model.length > 0 && encoder.encode(model).length <= 256);
    // Go includes old active/pending charges outside the rolling window. Rust
    // cannot express that exemption without changing its spending engine. Refuse
    // those obligations instead of aging them out or restarting their clock.
    require(!pending || (old.cost_micros ?? old.reserved_micros) === 0 || when > created - 86400000, 'old-pending-usage');
    require(!app.spend.some(spend => spend.id === spendId));
    // Rust keeps the nominal reservation even after settlement; known cost
    // always takes precedence. Go clears the reservation on measured settlement.
    // Keep exact cost (including zero), identity and time, with original fields
    // byte-preserved in custody. Unknown amounts must match the native reserve.
    app.spend.push({id: spendId, work_id: workId, created_ms: when,
      reserved_micros: 500000, cost_micros: old.cost_micros,
      status: old.cost_micros === null ? 'unknown' : 'known', model});
  };
  visit(t('job_attempts'), old => {
    require(['active', 'unknown', 'settled'].includes(old.state), 'generation-state');
    const job = app.jobs[id('job', old.job_id)];
    require(job);
    paid(old, id('spend', old.token), job.id, old.started_at, job.model, old.state === 'active');
  });
  visit(t('content_assessments'), old => {
    require(old.purpose === 'critic' && old.policy_version === 'critic-v1', 'critic-policy');
    require(['pending', 'judged', 'failed'].includes(old.status) && [0, 1].includes(old.transmissions));
    const attempt = unique(by(t('job_attempts'), 'token', old.attempt_token));
    require(attempt.job_id === old.job_id, 'critic-ownership');
    const job = app.jobs[id('job', old.job_id)];
    require(job);
    if (old.transmissions === 0) {
      // Staged or rejected before send: no paid obligation and no runnable work.
      // Original criticism/candidates/results remain in the verbatim Go archive.
      require(old.reserved_micros === 0 && (old.cost_micros === null || old.cost_micros === 0));
      return;
    }
    // Every sent row, including failed/retried candidates, keeps its own usage.
    // Attach to the existing owning job, never recreate a critic send lease.
    paid(old, id('critic-spend', old.id), job.id, old.created_at, old.request_model, old.status === 'pending');
  });
  for (let start = 0, index = 0; start < originalBytes.length; start += 48 * 1024, index++) {
    app.revival_go[originalHash + '/' + String(index).padStart(8, '0')] = base64(originalBytes.subarray(start, start + 48 * 1024));
  }
  const archive = {format: Object.keys(app.questions).length ? 'scry-rust-durable-sqlite-go-projection-v1' :
    'scry-rust-durable-sqlite-go-custody-v1', schema: 1, algorithm,
    created_ms: created, app, photos};
  require(encoder.encode(JSON.stringify(archive)).length <= 16 * 1024 * 1024);
  if (incomplete) throw new Error('DIAGNOSTIC_INCOMPLETE');
  return {archive, counts: {quizzes: t('quizzes').length, questions: Object.keys(app.questions).length,
    created_legacy_concepts: createdConcepts, created_legacy_notes: createdNotes,
    goals: Object.keys(app.goals).length, concepts: Object.keys(app.concepts).length,
    notes: Object.values(app.concepts).reduce((n, c) => n + c.notes.length, 0),
    content_versions: t('quiz_versions').length, events: app.events.length, overrides: app.overrides.length,
    receipts: Object.keys(app.operations).length, photos: photos.length, spend: app.spend.length}};
}
