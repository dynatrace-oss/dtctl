# Demo: agent diagnoses pizza service slowness

## Prompt to give the fresh agent

> My pizza service is slow. The process group instance is
> `PROCESS_GROUP_INSTANCE-82CBCAA0F214B356` on environment `hzi4275d`.
> I have `dtctl` available. Figure out why.

## What the agent should do (without any hints)

1. Run `dtctl exec --help` or `dtctl commands --brief -o json` to discover available commands
2. Notice `exec profile` and read its help
3. Run:
   ```
   dtctl exec profile -k hotspots \
     -e PROCESS_GROUP_INSTANCE-82CBCAA0F214B356 \
     --last 1h \
     --app my.dynatrace.profiling.timur.berdiev
   ```
4. Read `hotPaths` in the response and identify the root cause
