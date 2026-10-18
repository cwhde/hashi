# AUD-155: Latency chart effect repeatedly recreates itself

Priority: P1

Category: Frontend correctness / monitoring

Status: open; confirmed by manual source trace and independent reproduction.

## Finding

MonitorLatencyChart stores chart as reactive state. Its effect reads chart to destroy it and then assigns a new uPlot instance, retriggering the same effect. With nonempty data this repeatedly constructs/destroys charts until runtime failure or resource exhaustion. The actual component compiled with the installed Svelte version reproduced13 constructions/12 destroys before an independent safety bound deliberately stopped it.

## Evidence

web/src/lib/components/monitoring/MonitorLatencyChart.svelte:16-60; independent work/repros/chart-setup.mjs compiles the actual component and stubs only the plotting constructor/CSS in a jsdom DOM. Result: work/chart-probe.json. The inherited status tests mock this component and cannot detect the loop.

## Expected outcome and acceptance criteria

Keep the instance outside reactive dependency tracking or use effect cleanup with explicit data/size dependencies. One mount/data update must construct/update predictably, with exactly one final destroy and no self-triggering loop.
