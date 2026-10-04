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

# Where the dynatrace-for-ai checkout lives (its skills/dt-* are installed)
export EVAL_DFAI_DIR=$HOME/src/dynatrace-for-ai

# Optional
# export EVAL_MODEL=sonnet           # investigator
# export EVAL_JUDGE_MODEL=opus       # blind judge
# export EVAL_PARALLEL=6
# export EVAL_RUNS_DIR=/tmp/dtctl-recipes-evals   # NOT under $HOME (see README)
