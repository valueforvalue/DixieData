#!/usr/bin/env node
// scripts/backfill-labels.node.mjs
//
// Reads open-issue JSON on stdin, emits one line per issue:
//   SKIP|<num>|<existing labels>|<title>
//   ASSIGN|<num>|<area>|<priority>|<title>
// The shell wrapper applies the labels via `gh issue edit`.

let raw = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (c) => (raw += c));
process.stdin.on('end', () => {
  const issues = JSON.parse(raw);
  const TAG_RE = /\b(area:[a-z-]+|priority:[a-z-]+)\b/;
  const HIGH = /\b(crash|lost data|data loss|white screen|cannot|broken|fatal|regression|panic|deadlock|corrupt)\b/i;
  const LOW = /\b(polish|nice to have|eventually|cosmetic|optional|tweak)\b/i;

  function deriveArea(title, body, existing) {
    const m = title.match(/^\s*(feat|fix|ux|refactor|chore|perf|docs)\(([^)]+)\)/i);
    if (m) {
      const tag = m[2].toLowerCase();
      if (tag === 'cli') return 'area:cli';
      if (['share','export','import','tags'].includes(tag)) return `area:${tag}`;
      if (['nav','js','web','ui','layout','theme','browse','review-queue','preview','datepicker'].includes(tag)) return 'area:frontend';
      if (tag === 'templates') return 'area:templates';
      if (['routes','http','records','archive','appshell','dispatch','jobs'].includes(tag)) return 'area:backend';
      if (tag === 'schema') return 'area:db';
      if (tag === 'docs') return 'area:docs';
    }
    if (/^\s*docs\(/i.test(title)) return 'area:docs';
    const text = `${title} ${body}`;
    if (/\bcli\b|subcommand|dixiedata [a-z]+/i.test(text)) return 'area:cli';
    if (/pdf|jpg|csv|ical|\bexport\b/i.test(text)) return 'area:export';
    if (/ddbak|ddshare|\bimport\b/i.test(text)) return 'area:import';
    if (/\btag\b|virtual cemetery/i.test(text)) return 'area:tags';
    if (/templ|typst/i.test(text)) return 'area:templates';
    if (/\/share/i.test(text)) return 'area:share';
    if (/htmx|frontend|button/i.test(text)) return 'area:frontend';
    if (/migrat|schema|sqlite|\bsql\b/i.test(text)) return 'area:db';
    return 'area:backend';
  }

  function derivePriority(title, body) {
    const text = `${title} ${body}`;
    if (HIGH.test(text)) return 'priority:high';
    if (LOW.test(text)) return 'priority:low';
    return 'priority:medium';
  }

  for (const issue of issues) {
    const labels = issue.labels.map((l) => l.name);
    if (labels.some((l) => TAG_RE.test(l))) {
      console.log(`SKIP|${issue.number}|${labels.join(';')}|${issue.title}`);
      continue;
    }
    const area = deriveArea(issue.title, issue.body || '', labels);
    const pri = derivePriority(issue.title, issue.body || '');
    console.log(`ASSIGN|${issue.number}|${area}|${pri}|${issue.title}`);
  }
});