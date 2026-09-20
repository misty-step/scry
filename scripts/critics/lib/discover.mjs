/**
 * Live control discovery: aria + dom scans with agreement rules
 *
 * observation shape:
 * {
 *   url, title, html: string,
 *   aria: [{ role, name, level?, disabled, children?: [...] }],
 *   dom: [{ tag, role, type, name, level?, visible: boolean, inDetails: boolean }]
 * }
 *
 * Returns:
 * {
 *   question: { name: string, status: 'confirmed' | 'unverified' | 'missing' },
 *   answerAffordance: { type: 'choice' | 'recall' | 'next', controls: [...], status: 'confirmed' | 'unverified' | 'missing' },
 *   state: 'ungraded' | 'graded' | 'empty' | 'unknown',
 *   notes: string[]
 * }
 */

import { URL } from 'node:url';

// normalize names for comparison
export function normalizeName(name) {
  return String(name ?? '')
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim();
}

// role family mapping
function roleFamily(role) {
  if (!role) return 'unknown';
  if (role.startsWith('heading')) return 'heading';
  const map = {
    button: 'button', textbox: 'textbox', checkbox: 'checkbox', radio: 'radio',
    link: 'link', group: 'group', status: 'status', heading: 'heading',
    main: 'main', banner: 'banner'
  };
  return map[role] || role;
}

// parse aria snapshot yaml -> flattened nodes with parent reference
export function parseAriaSnapshot(yaml) {
  const nodes = [];
  let parentIndex = -1;

  for (const line of yaml.split('\n')) {
    const indent = line.search(/\S/);
    if (indent < 0) continue;
    const depth = Math.floor(indent / 2);
    const content = line.slice(indent + 2);

    // skip /url and similar properties
    if (content.startsWith('/')) continue;

    // match role with optional name and attributes
    const m = content.match(/^([a-z-]+)(?:\s+"((?:[^"\\]|\\.)*)")?(?:\s+\[([^\]]*)\])?:?$/);
    if (!m) continue;

    const role = m[1];
    const name = m[2] || '';

    let level = null;
    if (m[3]) {
      const lvl = m[3].match(/level=(\d+)/);
      if (lvl) level = parseInt(lvl[1], 10);
    }

    // find parent based on depth
    let parent = parentIndex;
    if (depth === 0) parent = -1;
    else {
      // search backwards for node with depth - 1
      for (let i = nodes.length - 1; i >= 0; i--) {
        if (nodes[i].depth === depth - 1) { parent = i; break; }
      }
    }

    nodes.push({ role, name, level, depth, parent, children: [] });
    if (parent >= 0) nodes[parent].children.push(nodes.length - 1);
    parentIndex = nodes.length - 1;
  }

  return nodes;
}

// discover controls
export function discoverControls(observation) {
  const { aria: ariaList, dom: domList, html } = observation;
  const notes = [];

  // find heading level 1
  const ariaH1 = (ariaList ?? []).find(n => n.role === 'heading' && n.level === 1);
  const domH1 = (domList ?? []).find(n => n.tag === 'h1' && n.visible);

  const questionName = normalizeName(ariaH1?.name || domH1?.name);
  let questionStatus = 'unverified';
  if (ariaH1 && domH1 && normalizeName(ariaH1.name) === normalizeName(domH1.name)) {
    questionStatus = 'confirmed';
  }

  // find answer affordance - check for choice buttons or recall textarea
  const choiceButtons = (domList ?? [])
    .filter(n => n.tag === 'button' && n.visible && !n.inDetails && n.class?.includes('choice'));
  const choiceFieldset = (domList ?? []).some(n => n.tag === 'fieldset' && n.visible && n.class?.includes('choice-fieldset'));
  const recallTextarea = (domList ?? [])
    .filter(n => n.tag === 'textarea' && n.visible && !n.inDetails && (n.fieldName === 'answer' || n.name === 'answer'));
  const answerForm = (domList ?? []).some(n => n.tag === 'form' && n.visible && !n.inDetails && n.class?.includes('answer-form'));

  const answerAffordance = { type: null, controls: [], status: 'unverified' };

  // Two-method agreement for choice
  if (choiceFieldset && choiceButtons.length >= 2 && choiceButtons.length <= 8) {
    answerAffordance.type = 'choice';
    answerAffordance.controls = choiceButtons.map(b => ({ name: normalizeName(b.name) }));
    answerAffordance.status = 'confirmed';
  }
  // Two-method agreement for recall
  else if (recallTextarea.length >= 1 && answerForm) {
    answerAffordance.type = 'recall';
    answerAffordance.controls = [{ type: 'textbox' }, { type: 'submit' }];
    answerAffordance.status = 'confirmed';
  }
  // Both methods agree no affordance exists -> missing
  else if (!choiceFieldset && choiceButtons.length === 0 && !recallTextarea.length && !answerForm) {
    answerAffordance.status = 'missing';
  }

  // detect graded state
  // Graded = status region + Next form confirmed, NO choice/recall affordance
  const statusRegion = (domList ?? [])
    .filter(n => n.role === 'status' && n.visible);
  const nextForm = (domList ?? [])
    .some(n => n.tag === 'form' && n.visible && !n.inDetails && n.dataset && Object.prototype.hasOwnProperty.call(n.dataset, 'next'));

  const hasStatus = statusRegion.length > 0;
  const hasNext = nextForm;
  const hasAnswerAffordance = answerAffordance.status === 'confirmed';

  // State determination
  let state = 'unknown';
  if (hasAnswerAffordance) {
    state = 'ungraded';
  } else if (hasStatus && hasNext && !hasAnswerAffordance) {
    state = 'graded';
  } else if (!hasAnswerAffordance && !hasStatus && !hasNext && (domList ?? []).some(n => n.tag === 'section' && n.class?.includes('empty-stage'))) {
    state = 'empty';
  }

  return {
    question: { name: questionName, status: questionStatus },
    answerAffordance,
    state,
    notes
  };
}
