# Copy to env.sh (git-ignored) and fill in. Everything tenant-specific lives
# here, never in tasks/ — task prompts reference these as ${EVAL_*}.

# dtctl contexts of the two tenants (must exist in your dtctl config; their
# credentials stay in the keyring and are looked up by token-ref).
export EVAL_T1=my-context-1          # OTel-native tenant: one EKS cluster, GenAI app, billing data
export EVAL_T2=my-context-2          # mixed OneAgent/OTel demo tenant: multi-cloud, RUM, RAP, synthetic

# Entities the task prompts name (tenant 2)
export EVAL_T2_CLUSTER=my-eks-cluster          # cluster with a crash-looping workload (t03)
export EVAL_T2_NAMESPACE=my-namespace          # namespace with Davis problems in 7d (t02)
export EVAL_T2_GENAI_MODEL="Example Model Pro" # model recorded by two instrumentations (t07)
export EVAL_T2_GENAI_MODEL_RE="example-model-pro" # substring matching its spellings (ground truth)

# Entities the task prompts name (tenant 1)
export EVAL_T1_AGENT_SERVICE=my-harness        # service whose ${EVAL_T1_AGENT_ENDPOINT} runs an LLM agent loop (t19)
export EVAL_T1_AGENT_ENDPOINT="POST /invoke"
export EVAL_T1_LOG_SERVICE=my-backend          # OTel service.name with partial k8s carriage on logs (t24)

# v2 tasks (tenant 2)
export EVAL_T2_PROBLEM_RC=P-0000001            # closed problem with a root cause and a deployment just before it (t25)
export EVAL_T2_CHANGE_TIME="02:00 UTC on 2026-01-01"  # when a workload started misbehaving after a spec change (t26)
export EVAL_T2_CHANGE_AT=2026-01-01T02:00:00Z  # the same instant, ISO (ground truth)
export EVAL_T2_CHANGE_WORKLOAD=my-workload
export EVAL_T2_CHANGE_NAMESPACE=my-namespace
export EVAL_T2_DEEP_SERVICE=my-agent-service   # ~0% failed requests, but failing outgoing/tool/LLM calls (t27)
export EVAL_T2_EXC_SERVICE=my-dotnet-service   # requests throw exceptions without being marked failed (t28)
export EVAL_T2_LOG_WORKLOAD=my-workload        # workload whose logs carry no workload/service field (t29)
export EVAL_T2_LOG_NAMESPACE=my-namespace
export EVAL_T2_LOG_CLUSTER=my-cluster
export EVAL_T2_SHIFT_SERVICE=my-proxy          # service whose endpoints slowed down in a past window (t31)
export EVAL_T2_SHIFT_FROM="2026-01-01 06:00"
export EVAL_T2_SHIFT_TO="2026-01-01 06:40"
export EVAL_T2_SHIFT_FROM_TS=2026-01-01T06:00:00Z
export EVAL_T2_SHIFT_TO_TS=2026-01-01T06:40:00Z
export EVAL_T2_PROBLEM_LAMBDA=P-0000002        # problem on a serverless function with a specific error in its logs (t37)

# v2 tasks (tenant 1)
export EVAL_T1_NEW_DAY=2026-01-01              # a UTC day with a new error template vs the 6 days before (t30)

# Where the dynatrace-for-ai skills live: a directory with skills/dt-*
# (a checkout, or a snapshot of the installed skills)
export EVAL_DFAI_DIR=$HOME/src/dynatrace-for-ai

# Optional
# export EVAL_MODEL=sonnet           # investigator (run.py --model overrides)
# export EVAL_JUDGE_MODEL=opus       # blind judge
# export EVAL_PARALLEL=6
# export EVAL_RUNS_DIR=/tmp/dtctl-recipes-evals   # NOT under $HOME (see README)
