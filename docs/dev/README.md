# Developer Documentation

This directory contains technical documentation for dtctl developers and contributors.

## Core Documentation

### [API_DESIGN.md](API_DESIGN.md)
Complete API design specification for dtctl, including:
- **Design Principles** - Philosophy and guidelines (kubectl-inspired, DX-first, AI-native)
- **Command Structure** - Verb-noun patterns, core commands, syntax
- **Resource Types** - All supported Dynatrace resources (dashboards, workflows, SLOs, etc.)
- **Common Operations** - CRUD patterns, filtering, output formats
- **Configuration & Context** - Multi-environment management
- **Advanced Features** - Wait conditions, watch mode, diff, templating

**Use this for**: Understanding the overall design, command patterns, and resource capabilities.

---

### [ARCHITECTURE.md](ARCHITECTURE.md)
Technical architecture and implementation details:
- **Technology Stack** - Go, Cobra, Viper, HTTP client libraries
- **Project Structure** - Directory layout, package organization
- **Development Workflow** - Setup, building, testing, CI/CD
- **Core Patterns** - Command handler pattern, resource handlers, output formatting
- **Dependencies** - Third-party libraries and rationale

**Use this for**: Setting up development environment, understanding code structure, adding new features.

---

### [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md)
Current implementation status and feature tracking:
- ✅ **Implemented Features** - Verbs, resources, special features
- 📊 **Resource Matrix** - Which operations are supported for each resource
- 🔮 **Future Features** - Link to planned features

**Use this for**: Checking what's already implemented, finding gaps, planning work.

---

### [FUTURE_FEATURES.md](FUTURE_FEATURES.md)
Detailed implementation plan for upcoming features:
- Platform Management
- State Management for Apps
- Grail Filter Segments, Fieldsets, Resource Store
- Feature Flags (with link to detailed spec)

**Use this for**: Understanding upcoming work, implementation priorities, detailed task breakdown.

---

### [FEATURE_FLAGS_API_DESIGN.md](FEATURE_FLAGS_API_DESIGN.md)
Comprehensive specification for Feature Flags API support:
- Resource hierarchy (projects, stages, flags, targeting)
- Complete command reference with examples
- Common workflows (progressive rollout, A/B testing, etc.)
- Manifest formats

**Use this for**: Understanding the complex Feature Flags API, implementation reference.

---

## Quick Reference

**New to dtctl development?** Start here:
1. Read [API_DESIGN.md](API_DESIGN.md) - Design Principles section
2. Review [ARCHITECTURE.md](ARCHITECTURE.md) - Setup your environment
3. Check [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) - See what's done

**Adding a new resource?** Follow this pattern:
1. Define in [API_DESIGN.md](API_DESIGN.md) - Resource Types section
2. Implement in `pkg/resources/<resource>/`
3. Add commands in `cmd/`
4. Update [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md)
5. Add tests in `test/`

**Proposing a new feature?**
1. Check [FUTURE_FEATURES.md](FUTURE_FEATURES.md) to avoid duplicates
2. Create detailed design spec (see FEATURE_FLAGS_API_DESIGN.md as example)
3. Add to future features plan

---

## Documentation Maintenance

- Keep docs in sync with code changes
- Update IMPLEMENTATION_STATUS.md when completing features
- Cross-link related sections
- Include examples and use cases
- Date significant updates
