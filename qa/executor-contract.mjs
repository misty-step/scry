export function deriveStories(frozen, changes) {
  const required = new Set(frozen.required_stories), selection = [];
  for (const change of changes) {
    const [status, ...paths] = change.split('\t');
    if (!/^[AMDTURC][0-9]*$/.test(status) || !paths.length || paths.some(path => !path)) throw Error('invalid Git diff record');
    for (const path of paths) {
      let stories;
      if (/^internal\/learning\/(learning|short|semantic)(_test)?\.go$/.test(path)) stories = ['US-003', 'US-008'];
      else if (path === 'CONTRACT.md') stories = ['US-008'];
      else stories = Object.keys(frozen.stories);
      if (status === 'D') stories = Object.keys(frozen.stories);
      selection.push({path, status, stories, reason: stories.length > 2 ? 'unknown or deleted path: full story set' : 'grading boundary and reverse consumers'});
      for (const id of stories) required.add(id);
    }
  }
  return {required_stories: [...required].sort(), selection};
}
