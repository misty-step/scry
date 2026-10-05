import assert from 'node:assert/strict';
import {test} from 'node:test';
import {deriveStories} from './executor-contract.mjs';
const frozen = {required_stories: ['US-008'], stories: {'US-001': ['one'], 'US-003': ['one'], 'US-008': ['one']}};
test('grading changes include their reverse consumers', () => {
  assert.deepEqual(deriveStories(frozen, ['M\tinternal/learning/short.go']).required_stories, ['US-003', 'US-008']);
});
test('unmapped changes and deleted tests select every story', () => {
  for (const change of ['M\tunknown.go', 'D\tinternal/learning/short_test.go']) assert.deepEqual(deriveStories(frozen, [change]).required_stories, ['US-001','US-003','US-008']);
});
test('renames account for both old and new paths', () => {
  assert.deepEqual(deriveStories(frozen, ['R100\tinternal/learning/short.go\tnew-place.go']).required_stories, ['US-001','US-003','US-008']);
});
