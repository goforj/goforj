# Vue 3 + TypeScript + Vite

This template should help get you started developing with Vue 3 and TypeScript in Vite. The template uses Vue 3 `<script setup>` SFCs, check out the [script setup docs](https://v3.vuejs.org/api/sfc-script-setup.html#sfc-script-setup) to learn more.

Learn more about the recommended Project Setup and IDE Support in the [Vue Docs TypeScript Guide](https://vuejs.org/guide/typescript/overview.html#project-setup).

## Monitoring Chart Tests

The uptime gopher monitoring charts have regressed in data shaping more often than in Vue rendering.

Current rule:

- keep chart windowing, dedupe, gap detection, and edge-carry behavior in a pure helper
- test that helper directly with scenario coverage
- keep broad component snapshot tests secondary

Current files:

- `src/lib/monitoring-chart.ts`
- `src/components/__tests__/monitoring-chart.test.ts`
- `src/lib/heartbeat-pills.ts`
- `src/components/__tests__/heartbeat-pills.test.ts`

Current command:

```bash
npm run test:monitoring-ui
```

Current scenarios covered:

- retry bursts should not fragment a normal `1h` line into isolated points
- real large gaps should still render holes
- unsorted input should render in ascending order
- duplicate timestamps should collapse deterministically
- `paused` and `maintenance` rows should normalize to zero latency
- small trailing gaps should carry to the window edge
- large leading gaps should stay holes instead of inventing continuity
- out-of-range rows should be excluded

Heartbeat pill coverage should focus on:

- oldest/newest ordering staying stable
- left-padding of missing history
- dropping the still-open newest unknown bucket
- status/point alignment when one payload is longer than the other
- deterministic trimming when the server returns more pills than the UI shows

Do not rely only on `monitor-detail.snapshot.test.ts` for this path. That snapshot suite is useful for broader UI baselines, but it is too indirect for chart-shaping regressions and is easier to break for unrelated Vue/plugin reasons.
