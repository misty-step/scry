import type { ReactElement } from 'react';

function splitInlineCodeSegments(text: string) {
  const segments: Array<{ type: 'text' | 'code'; value: string }> = [];
  let cursor = 0;

  while (cursor < text.length) {
    const codeStart = text.indexOf('`', cursor);
    if (codeStart === -1) {
      segments.push({ type: 'text', value: text.slice(cursor) });
      break;
    }

    if (codeStart > cursor) {
      segments.push({ type: 'text', value: text.slice(cursor, codeStart) });
    }

    const codeEnd = text.indexOf('`', codeStart + 1);
    if (codeEnd === -1) {
      segments.push({ type: 'text', value: text.slice(codeStart) });
      break;
    }

    segments.push({ type: 'code', value: text.slice(codeStart + 1, codeEnd) });
    cursor = codeEnd + 1;
  }

  return segments;
}

function renderEmphasis(text: string, keyPrefix: string) {
  const nodes: Array<string | ReactElement> = [];
  let cursor = 0;
  let key = 0;

  const pushPlainText = (value: string) => {
    if (!value) return;
    nodes.push(value);
  };

  while (cursor < text.length) {
    if (text.startsWith('**', cursor)) {
      const end = text.indexOf('**', cursor + 2);
      if (end !== -1) {
        nodes.push(
          <strong key={`${keyPrefix}-strong-${key++}`}>
            {renderEmphasis(text.slice(cursor + 2, end), `${keyPrefix}-s${key}`)}
          </strong>
        );
        cursor = end + 2;
        continue;
      }
    }

    if (text[cursor] === '*') {
      const end = text.indexOf('*', cursor + 1);
      if (end !== -1) {
        nodes.push(
          <em key={`${keyPrefix}-em-${key++}`}>
            {renderEmphasis(text.slice(cursor + 1, end), `${keyPrefix}-e${key}`)}
          </em>
        );
        cursor = end + 1;
        continue;
      }
    }

    const nextBold = text.indexOf('**', cursor);
    const nextItalic = text.indexOf('*', cursor);
    const nextTokenCandidates = [nextBold, nextItalic].filter((value) => value >= 0);
    const nextToken =
      nextTokenCandidates.length > 0 ? Math.min(...nextTokenCandidates) : text.length;

    if (nextToken <= cursor) {
      pushPlainText(text[cursor] ?? '');
      cursor += 1;
      continue;
    }

    pushPlainText(text.slice(cursor, nextToken));
    cursor = nextToken;
  }

  return nodes;
}

export function renderInlineMarkdown(text: string) {
  const nodes: Array<string | ReactElement> = [];

  splitInlineCodeSegments(text).forEach((segment, index) => {
    if (segment.type === 'code') {
      nodes.push(
        <code key={`code-${index}`} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em]">
          {segment.value}
        </code>
      );
      return;
    }

    nodes.push(...renderEmphasis(segment.value, `segment-${index}`));
  });

  return nodes;
}
