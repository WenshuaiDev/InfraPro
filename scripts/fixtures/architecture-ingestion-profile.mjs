// After the load clock has stopped, connect each durable handoff to its real
// domain attempt. This profile reads existing evidence and needs no database.
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

const directory = resolve(process.argv[2]);
const load = (file) => JSON.parse(readFileSync(file, 'utf8'));
const result = load(join(directory, 'comparison.json'));
const ingestion = dirname(dirname(directory));
const p95 = (values) => {
  assert.ok(values.length > 0 && values.every(Number.isFinite));
  return values.sort((a, b) => a - b)[Math.ceil(values.length * 0.95) - 1];
};
const rounds = [];
for (const round of result.rounds) {
  if (round.mode === 'a') continue;
  const raw = join(ingestion, round.raw_directory);
  const stages = load(join(raw, 'stage-attempts.json'));
  const deliveries = load(join(raw, 'event-deliveries.json'));
  const linked = {};
  for (const stage of ['identify', 'plan', 'parse']) {
    const observations = stages.filter((v) => v.stage === (stage === 'plan' ? 'plan_effect' : stage));
    const events = deliveries.filter((v) => v.stage === stage);
    const byAggregate = new Map(events.map((v) => [v.aggregate, v]));
    assert.equal(byAggregate.size, events.length, 'Expected one generation per fixture key');
    const afterHandoff = observations.map((o) => {
      const event = byAggregate.get(o.id);
      assert.ok(event, 'Missing durable work for a measured domain stage');
      return Date.parse(o.claimed) - Date.parse(event.first_handoff);
    });
    linked[stage] = {
      domain_keys: observations.length,
      event_assignments: events.length,
      first_broker: events.filter((v) => v.via === 'broker').length,
      first_pg: events.filter((v) => v.via === 'pg').length,
      redeliveries: events.reduce((sum, v) => sum + v.duplicates, 0),
      outbox_to_handoff_p95_ms: p95(events.map((v) => v.milliseconds)),
      domain_ready_to_claim_or_effect_p95_ms: p95(observations.map((v) => v.milliseconds)),
      handoff_to_domain_claim_or_effect_p95_ms: p95(afterHandoff),
      domain_started_before_handoff: afterHandoff.filter((ms) => ms < 0).length,
    };
    assert.equal(linked[stage].first_pg + linked[stage].first_broker, events.length);
  }
  rounds.push({ round: round.name, stages: linked });
}
writeFileSync(
  join(directory, 'stage-capacity.json'),
  JSON.stringify(
    {
      campaign: result.campaign,
      population: 'All recorded event assignments and corresponding distinct domain keys. Planning records its first committed effect; identify and parse use the real first attempt. Handoff is the first PG receipt. Event and domain populations are listed separately. Signed clock differences use millisecond precision.',
      rounds,
    },
    null,
    2,
  ) + '\n',
  { mode: 0o600 },
);
