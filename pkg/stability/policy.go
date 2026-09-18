package stability

import (
	"fmt"
	"strings"
)

// Policy is a deployment's stability constraint: an ordered floor plus a narrow
// list of per-command and per-flag exceptions to it.
//
// The exceptions exist because a single floor is too coarse for the motivating
// case. A team that wants one experimental command should not have to drop the
// floor, which would grant the *entire* experimental surface — including
// experimental flags added to already-allowed stable commands in a later
// release. And a tight command profile cannot substitute: profile matching is
// prefix-based and subtree-inclusive, so it is default-deny at a point in time
// but default-allow over time within an allowed subtree. Right for topics,
// wrong for a contract.
type Policy struct {
	// Floor is the weakest contract a command or flag may offer and still be
	// usable. The empty value means the default floor.
	Floor Level
	// Exceptions are targets admitted below the floor. Each is a command path,
	// optionally suffixed with one flag: "inventory",
	// "query --decode-snapshots". A command entry also admits the flags that
	// are below the floor only because that command is — see AllowsFlag.
	Exceptions []Exception
	// Development are the command paths whose development-tier opt-in has
	// already been given, subtree-inclusive. They are implicit exceptions, not
	// user-written ones.
	//
	// Without them the development tier could not work at all: the default
	// floor is experimental, so a feature the operator deliberately switched on
	// would be registered and then immediately blocked. The opt-in is
	// per-feature, out-of-band and strictly more explicit than a floor, so the
	// floor governs the tiers that ship *registered* — stable and experimental
	// — and leaves development to its own switch. Subtree-inclusive, because
	// the switch gates a whole feature rather than one command.
	Development []string
}

// Exception is one audited opt-in below the floor.
type Exception struct {
	// Command is the space-joined command path ("get workflows").
	Command string
	// Flag is the single flag name (without dashes) the exception admits, or
	// "" when the exception admits the command itself.
	Flag string
}

// String renders an exception in the form it is written in config.
func (e Exception) String() string {
	if e.Flag == "" {
		return e.Command
	}
	return e.Command + " --" + e.Flag
}

// ParseExceptions parses config entries into Exceptions. An entry is a command
// path optionally followed by a single `--flag` token. A malformed entry is an
// error rather than a silent skip: a typo in an exception would otherwise
// silently tighten the surface and produce a confusing block much later.
func ParseExceptions(entries []string) ([]Exception, error) {
	var out []Exception
	for _, raw := range entries {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		var cmdParts []string
		var flag string
		for _, f := range fields {
			if strings.HasPrefix(f, "--") {
				if flag != "" {
					return nil, fmt.Errorf(
						"stability exception %q names more than one flag; "+
							"write one entry per flag", raw)
				}
				flag = strings.TrimPrefix(f, "--")
				continue
			}
			if flag != "" {
				return nil, fmt.Errorf(
					"stability exception %q puts a command segment after a flag; "+
						"write the command path first, then one --flag", raw)
			}
			cmdParts = append(cmdParts, f)
		}
		if len(cmdParts) == 0 {
			return nil, fmt.Errorf(
				"stability exception %q names a flag without a command; "+
					"write e.g. \"query --decode-snapshots\"", raw)
		}
		out = append(out, Exception{Command: strings.Join(cmdParts, " "), Flag: flag})
	}
	return out, nil
}

// EffectiveFloor returns the floor, substituting DefaultFloor when unset.
func (p Policy) EffectiveFloor() Level {
	if p.Floor == "" {
		return DefaultFloor
	}
	return p.Floor
}

// RequiredFor returns the floor that applies to one target: the policy floor,
// unless the target sits under an enabled development feature, or an exception
// names this exact command (for a command target) or this exact command+flag
// (for a flag target) — in which case the floor drops to development for that
// target only.
//
// The exception is a *parameterization of the floor stage*, not an override of
// another axis — so the invariant that no later stage widens what an earlier
// one narrowed still holds. In particular an exception cannot resurrect an
// unregistered development command: registration happens first.
func (p Policy) RequiredFor(command, flag string) Level {
	for _, granted := range p.Development {
		if segmentPrefix(granted, command) {
			return Development
		}
	}
	for _, e := range p.Exceptions {
		if e.Command != command {
			continue
		}
		// Exact match only: a command-target exception admits the command, a
		// flag-target exception admits that flag. AllowsFlag adds the one case
		// this cannot see — a flag that is below the floor purely by
		// inheritance.
		if e.Flag == flag {
			return Development
		}
	}
	return p.EffectiveFloor()
}

// AllowsCommand reports whether a command at `path` with effective level `lvl`
// is usable under this policy.
func (p Policy) AllowsCommand(path string, lvl Level) bool {
	return lvl.AtLeast(p.RequiredFor(path, ""))
}

// AllowsFlag reports whether a flag on the command at `path` is usable under
// this policy. `effective` is the flag's level as callers experience it
// (including what it inherits from its command); `own` is the level the flag
// declares for itself.
func (p Policy) AllowsFlag(path, flag string, effective, own Level) bool {
	if effective.AtLeast(p.RequiredFor(path, flag)) {
		return true
	}
	// The flag declares nothing weaker of its own, so it is below the floor
	// only because its command is — and then it stands or falls with the
	// command. Requiring a separate entry would mean an exception for
	// `inventory` had to enumerate all five of its flags, and a flag added in
	// a later release would silently break the caller that did: the opposite of
	// what an audited, explicit grant is for.
	//
	// A flag with its own weaker mark (`query --decode-snapshots` on a stable
	// `query`) still needs its own entry. There the flag is the risk being
	// accepted, not the command, which is the whole point of per-flag
	// granularity.
	if own == Default {
		// own == Default means effective is exactly the command's level.
		return p.AllowsCommand(path, effective)
	}
	return false
}

// Restricts reports whether the policy can block anything at all. A policy at
// the weakest floor with no exceptions is a no-op, and the caller can skip the
// tree walk entirely.
func (p Policy) Restricts() bool {
	return p.EffectiveFloor().Rank() > Development.Rank()
}

// ExceptionStrings renders the exceptions for the catalog, so an agent that
// reads `min_stability: stable` and finds an experimental command in the tree
// has a consistent picture of its own constraints.
func (p Policy) ExceptionStrings() []string {
	if len(p.Exceptions) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.Exceptions))
	for _, e := range p.Exceptions {
		out = append(out, e.String())
	}
	return out
}
