package grokconfig

func f(key, label, typ, desc string, def any, opts ...string) Field {
	field := Field{Key: key, Label: label, Type: typ, Description: desc, Default: def}
	if typ == TypeEnum {
		field.Options = opts
	}
	return field
}

func secret(field Field) Field {
	field.Secret = true
	return field
}

func ph(field Field, placeholder string) Field {
	field.Placeholder = placeholder
	return field
}

func rng(field Field, min, max float64) Field {
	field.Min = &min
	field.Max = &max
	return field
}

func Groups() []Group {
	return []Group{
		{ID: "models", Title: "Models", Description: "Default model, custom endpoints, and shared providers"},
		{ID: "mcp", Title: "MCP servers", Description: "External tools over the Model Context Protocol"},
		{ID: "agent", Title: "Agent & tools", Description: "Permissions, sandbox, session, and built-in tools"},
		{ID: "ui", Title: "Appearance", Description: "TUI behavior, scrolling, and notifications"},
		{ID: "auth", Title: "Auth & endpoints", Description: "Login providers and API base URLs"},
		{ID: "memory", Title: "Memory", Description: "Cross-session memory index"},
		{ID: "extras", Title: "Skills & agents", Description: "Skills, plugins, subagents, and workflows"},
		{ID: "advanced", Title: "Advanced", Description: "CLI, telemetry, compatibility, and environment policy"},
	}
}

func modelFields() []Field {
	return []Field{
		ph(f("model", "API model ID", TypeString, "Identifier sent to the provider API. If omitted, the section name is used.", nil), "gpt-4o"),
		ph(f("name", "Display name", TypeString, "Shown in the model picker.", nil), "GPT-4o"),
		f("description", "Description", TypeString, "Optional picker description.", nil),
		ph(f("base_url", "Base URL", TypeString, "OpenAI-compatible API root, including /v1 when required.", nil), "https://api.example.com/v1"),
		f("model_provider", "Provider ID", TypeString, "Inherit connection settings from [model_providers.<id>].", nil),
		f("api_backend", "API backend", TypeEnum, "Wire protocol for this model.", "chat_completions", "chat_completions", "responses", "messages"),
		secret(ph(f("api_key", "API key", TypeString, "Stored in config.toml. Prefer env_key when you can.", nil), "sk-…")),
		f("env_key", "API key env var", TypeStringList, "Environment variable(s) holding the API key. First set, non-empty value wins.", nil),
		f("auth_provider", "Auth provider", TypeString, "Named credential helper for rotating tokens.", nil),
		rng(f("temperature", "Temperature", TypeFloat, "Sampling temperature (0–2). Per-model value wins over [models] default.", nil), 0, 2),
		rng(f("top_p", "Top P", TypeFloat, "Nucleus sampling parameter.", nil), 0, 1),
		f("max_completion_tokens", "Max completion tokens", TypeInt, "Maximum tokens per response.", nil),
		f("context_window", "Context window", TypeInt, "Total context window in tokens (used for auto-compact). Default 200000 for unknown models.", nil),
		f("max_retries", "Max retries", TypeInt, "HTTP retry budget for this model.", nil),
		f("inference_idle_timeout_secs", "Idle timeout (seconds)", TypeInt, "Abort a hung inference stream after this many seconds.", nil),
		f("stream_tool_calls", "Stream tool calls", TypeBool, "Affects request shape. Some BYOK endpoints need this left unset or false.", nil),
		f("supports_reasoning_effort", "Supports reasoning effort", TypeBool, "Advertise reasoning-effort controls for this model.", nil),
		f("reasoning_efforts", "Effort levels", TypeStringList, "Levels offered in /effort and the model picker. Must include every level you want (e.g. max); models reject unlisted levels.", nil),
		f("supports_backend_search", "Supports backend search", TypeBool, "Required if this model is used for server-side web_search.", nil),
		f("extra_headers", "Extra headers", TypeMap, "Headers sent verbatim with every inference request. Use header = value.", nil),
		f("query_params", "Query params", TypeMap, "Appended to every request URL. Do not put secrets here.", nil),
		f("env_http_headers", "Headers from env", TypeMap, "Maps a request header to an environment variable name.", nil),
	}
}

func providerFields() []Field {
	return []Field{
		ph(f("name", "Display name", TypeString, "Optional label for this provider.", nil), "OpenAI"),
		ph(f("api_base_url", "API base URL", TypeString, "Shared OpenAI-compatible root inherited by models that set model_provider.", nil), "https://api.openai.com/v1"),
		ph(f("base_url", "Base URL (alias)", TypeString, "Accepted as an alias of api_base_url.", nil), "https://api.openai.com/v1"),
		f("api_backend", "API backend", TypeEnum, "Default wire protocol for models using this provider.", "chat_completions", "chat_completions", "responses", "messages"),
		secret(ph(f("api_key", "API key", TypeString, "Shared key. Prefer env_key when you can.", nil), "sk-…")),
		f("env_key", "API key env var", TypeStringList, "Environment variable(s) holding the API key. First set, non-empty value wins.", nil),
		f("auth_provider", "Auth provider", TypeString, "Named credential helper for rotating tokens.", nil),
		f("extra_headers", "Extra headers", TypeMap, "Inherited by models that do not set their own extra_headers keys.", nil),
		f("query_params", "Query params", TypeMap, "Inherited when a model sets none of its own.", nil),
		f("env_http_headers", "Headers from env", TypeMap, "Maps a request header to an environment variable name.", nil),
	}
}

func mcpFields() []Field {
	return []Field{
		ph(f("command", "Command", TypeString, "Local stdio server executable. Leave empty for HTTP/SSE servers.", nil), "npx"),
		f("args", "Arguments", TypeStringList, "Command-line arguments for the stdio server.", nil),
		ph(f("url", "URL", TypeString, "HTTP or SSE endpoint. Prefer this over wrapping a remote server in npx mcp-remote.", nil), "https://mcp.example.com/mcp"),
		f("env", "Environment", TypeMap, "Environment variables for the stdio process. Use VAR = value.", nil),
		f("headers", "HTTP headers", TypeMap, "Headers for remote servers. {{session_id}} is expanded.", nil),
		f("enabled", "Enabled", TypeBool, "Disable without deleting the server.", true),
		f("startup_timeout_sec", "Startup timeout (seconds)", TypeInt, "Init timeout. Default 30. Cold npx/uvx servers often need more.", 30),
		f("tool_timeout_sec", "Tool timeout (seconds)", TypeInt, "Fallback per-tool-call timeout.", 6000),
		f("tool_timeouts", "Per-tool timeouts", TypeIntMap, "Override timeout for specific tools, in seconds. tool_name = 120", nil),
	}
}

func Sections() []Section {
	return []Section{
		{
			ID: "models", Group: "models", Title: "Model defaults",
			Description: "Applied to every model unless a [model.<id>] entry overrides the same key.",
			Fields: []Field{
				ph(f("models.default", "Default model", TypeString, "Model used for new sessions.", "grok-4.5"), "grok-4.5"),
				ph(f("models.web_search", "Web search model", TypeString, "Model used by the web_search tool. Custom targets also need a [model.*] entry.", "grok-4.5"), "grok-4.5"),
				f("models.default_reasoning_effort", "Default reasoning effort", TypeEnum, "Default reasoning effort for models that support it. Models only accept the levels they advertise.", nil, "none", "minimal", "low", "medium", "high", "xhigh", "max"),
				rng(f("models.temperature", "Temperature", TypeFloat, "Default sampling temperature.", nil), 0, 2),
				rng(f("models.top_p", "Top P", TypeFloat, "Default nucleus sampling.", nil), 0, 1),
				f("models.max_completion_tokens", "Max completion tokens", TypeInt, "Default max tokens per response.", 8192),
				f("models.max_retries", "Max retries", TypeInt, "Default HTTP retry budget.", 8),
				f("models.inference_idle_timeout_secs", "Idle timeout (seconds)", TypeInt, "Default hung-stream abort timeout.", 600),
				f("models.stream_tool_calls", "Stream tool calls", TypeBool, "Default request shape. Some BYOK endpoints need this unset.", true),
				f("models.extra_headers", "Extra headers", TypeMap, "Base headers for every model. Per-model keys win (case-insensitive).", nil),
			},
		},
		{
			ID: "mcp", Group: "mcp", Title: "MCP defaults",
			Description: "Global MCP tool-result cap. Per-server settings live in the list below.",
			Fields: []Field{
				f("mcp.max_output_bytes", "Max tool output (bytes)", TypeInt, "Large MCP results are truncated inline. Default 20000.", 20000),
			},
		},
		{
			ID: "features", Group: "agent", Title: "Features",
			Fields: []Field{
				f("features.telemetry", "Telemetry", TypeBool, "Anonymous product-analytics master switch.", false),
				f("features.feedback", "Feedback", TypeBool, "In-product feedback system.", true),
				f("features.lsp_tools", "LSP tools", TypeBool, "Expose the lsp tool. Passive diagnostics still work from lsp.json.", false),
				f("features.codebase_indexing", "Codebase indexing", TypeBool, "Code graph indexing.", true),
				f("features.two_pass_compaction", "Two-pass compaction", TypeBool, "Prefire two-pass compaction (opt-in).", false),
				f("features.remote_fetch", "Remote catalog fetch", TypeBool, "Allow online model-catalog fetches. Disable for air-gapped hosts.", true),
				f("features.managed_config", "Managed config sync", TypeBool, "Background managed-config sync. Independent of remote_fetch.", true),
				f("features.support_permission", "Support permission prompts", TypeBool, "Prompt before tool execution (legacy name used in some builds).", false),
			},
		},
		{
			ID: "session", Group: "agent", Title: "Session",
			Fields: []Field{
				rng(f("session.auto_compact_threshold_percent", "Auto-compact threshold (%)", TypeInt, "Compact when the context window reaches this percent.", 85), 1, 100),
				f("session.load_envrc", "Load .envrc", TypeBool, "Load .envrc environment variables into bash commands.", true),
			},
		},
		{
			ID: "tools", Group: "agent", Title: "Tools",
			Fields: []Field{
				f("tools.respect_gitignore", "Respect gitignore", TypeBool, "Skip gitignored files in every tool. Also GROK_RESPECT_GITIGNORE.", false),
			},
		},
		{
			ID: "toolset", Group: "agent", Title: "Toolset",
			Fields: []Field{
				f("toolset.bash.timeout_secs", "Bash timeout (seconds)", TypeFloat, "Foreground command timeout.", 120.0),
				f("toolset.bash.output_byte_limit", "Bash output limit (bytes)", TypeInt, "Max captured command output.", 20000),
				f("toolset.ask_user_question.timeout_enabled", "Ask-question timeout", TypeBool, "Wait a limited time for questionnaire answers.", true),
				f("toolset.ask_user_question.timeout_secs", "Ask-question timeout (seconds)", TypeInt, "Seconds to wait when the timeout is enabled. Default 1800 (30 min).", 1800),
				ph(f("toolset.web_fetch.proxy_endpoint", "Web fetch proxy", TypeString, "Egress proxy URL for web_fetch.", nil), "https://proxy.example.com"),
				f("toolset.web_fetch.allowed_domains", "Web fetch allowlist", TypeStringList, "Override the built-in domain allowlist.", nil),
				f("toolset.web_fetch.allow_local", "Allow localhost fetch", TypeBool, "Allow web_fetch to explicit loopback hosts only. Private/metadata stay blocked.", false),
			},
		},
		{
			ID: "sandbox", Group: "agent", Title: "Sandbox",
			Fields: []Field{
				f("sandbox.profile", "Default sandbox profile", TypeEnum, "OS-level isolation for the agent and child commands. Off by default.", "off", "off", "workspace", "devbox", "read-only", "strict"),
			},
		},
		{
			ID: "permission", Group: "agent", Title: "Permission rules",
			Description: "Compact allow/deny/ask lists. Deny always wins. Same syntax as --allow / --deny.",
			Fields: []Field{
				f("permission.allow", "Allow", TypeStringList, "Always-allow rules, e.g. Bash(git *) or Edit(/tmp/**).", nil),
				f("permission.ask", "Ask", TypeStringList, "Always prompt for these rules.", nil),
				f("permission.deny", "Deny", TypeStringList, "Hard blocks, e.g. Bash(rm -rf *). Wins over always-approve.", nil),
			},
		},
		{
			ID: "agent", Group: "agent", Title: "Default agent",
			Fields: []Field{
				ph(f("agent.name", "Agent name", TypeString, "Discovered agent profile name.", nil), "grok-build"),
				ph(f("agent.definition", "Agent definition path", TypeString, "Explicit path to an agent .md file. Alternative to name.", nil), "/path/to/agent.md"),
			},
		},
		{
			ID: "ui", Group: "ui", Title: "Appearance & permissions",
			Fields: []Field{
				f("ui.simple_mode", "Readline prompt editing", TypeBool, "true = readline prompt editing (default). false = experimental vim editing in the prompt.", true),
				f("ui.vim_mode", "Vim scrollback navigation", TypeBool, "Vim-style keys in the scrollback pane. Independent of simple_mode.", false),
				f("ui.compact_mode", "Compact mode", TypeBool, "Denser TUI layout. Also /compact-mode.", false),
				f("ui.max_thoughts_width", "Max thoughts width", TypeInt, "Max column width for reasoning display.", 120),
				f("ui.show_thinking_blocks", "Show thinking blocks", TypeBool, "Show agent thinking in the TUI.", true),
				f("ui.group_tool_verbs", "Group tool verbs", TypeBool, "Fold runs of read/search/list calls into one row.", true),
				f("ui.collapsed_edit_blocks", "Collapsed edit blocks", TypeBool, "Show edits as one-line +N/−M summaries.", false),
				f("ui.page_flip_on_send", "Snap prompt to top on send", TypeBool, "Pin a just-sent prompt at the top of the viewport.", true),
				f("ui.screen_mode", "Default screen mode", TypeEnum, "Sticky render mode for plain grok launches. Restart required.", "fullscreen", "fullscreen", "minimal"),
				f("ui.permission_mode", "Permission mode", TypeEnum, "Default approval policy. always-approve is the native name for --yolo.", "ask", "ask", "auto", "always-approve", "acceptEdits", "bypassPermissions"),
				f("ui.yolo", "Always-approve (legacy yolo)", TypeBool, "Legacy alias for always-approve. Prefer permission_mode.", false),
				f("ui.default_selected_permission", "Default selected permission", TypeEnum, "Highlighted row on the first approval prompt of a session.", "always_allow_all_sessions", "always_allow_all_sessions", "allow_command_always", "allow_once", "reject"),
				f("ui.remember_tool_approvals", "Remember tool approvals", TypeBool, "Show per-command Always allow options. Grants are per project.", false),
				f("ui.disable_bypass_permissions_mode", "Disable always-approve", TypeBool, "Lock always-approve off (usually set in requirements.toml).", false),
				rng(f("ui.scroll_speed", "Scroll speed", TypeInt, "Wheel/trackpad speed. 50 = 1.0×, 1 = 0.1×, 100 = 6.0×.", 50), 1, 100),
				f("ui.scroll_mode", "Scroll input", TypeEnum, "Force wheel vs trackpad when auto-detection misreads the device.", "auto", "auto", "wheel", "trackpad"),
				rng(f("ui.scroll_lines", "Scroll lines", TypeInt, "Lines per scroll tick. Unset keeps the terminal profile in charge.", nil), 1, 10),
				f("ui.invert_scroll", "Invert scroll", TypeBool, "Natural scrolling.", false),
				f("ui.show_timeline", "Show timeline", TypeBool, "Show the session timeline.", nil),
				f("ui.show_timestamps", "Show timestamps", TypeBool, "Show timestamps on scrollback entries.", false),
				ph(f("ui.fork_secondary_model", "Fork secondary model", TypeString, "Model used for forked/secondary work.", nil), "grok-4.6"),
				ph(f("ui.theme", "Theme", TypeString, "TUI theme name, if configured.", nil), "default"),
			},
		},
		{
			ID: "notifications", Group: "ui", Title: "Notifications",
			Fields: []Field{
				f("ui.notifications.method", "Method", TypeEnum, "Terminal notification protocol. auto picks the best for your terminal.", "auto", "auto", "osc9", "osc99", "osc777", "bel", "none"),
				f("ui.notifications.condition", "When", TypeEnum, "unfocused fires only after the terminal loses focus.", "unfocused", "unfocused", "always", "never"),
				f("ui.notifications.idle_threshold_secs", "Idle threshold (seconds)", TypeInt, "Minimum seconds unfocused before a notification fires.", 3),
				f("ui.notifications.events", "Events", TypeStringList, "turn_complete, approval_required, session_ready, task_complete, agent_error.", []any{"turn_complete", "approval_required"}),
				f("ui.notifications.sleep_prevention", "Prevent display sleep", TypeBool, "Keep the display awake during agent turns.", true),
				f("ui.notifications.progress_bar", "Tab progress bar", TypeBool, "OSC 9;4 progress in the terminal tab.", true),
				f("ui.notifications.title.enabled", "Set terminal title", TypeBool, "Reflect agent state in the terminal title.", true),
				f("ui.notifications.title.items", "Title items", TypeStringList, "action-required, spinner, activity, session-name, cwd, model, turn-timer, grok.", nil),
			},
		},
		{
			ID: "auth", Group: "auth", Title: "Authentication",
			Fields: []Field{
				ph(f("auth.auth_provider_command", "Auth provider command", TypeString, "External binary that prints a token on stdout.", nil), "/usr/local/bin/my-auth-provider"),
				ph(f("auth.auth_provider_label", "Auth provider label", TypeString, "Display name on the TUI login screen.", nil), "Acme Corp"),
				f("auth.auth_token_ttl", "Token TTL (seconds)", TypeInt, "Assumed lifetime for bare tokens from the provider.", 3600),
			},
		},
		{
			ID: "oidc", Group: "auth", Title: "OIDC",
			Fields: []Field{
				ph(f("grok_com_config.oidc.issuer", "Issuer", TypeString, "OIDC issuer URL.", nil), "https://acme.okta.com"),
				ph(f("grok_com_config.oidc.client_id", "Client ID", TypeString, "Public OIDC client id (PKCE, no secret).", nil), "0oa…"),
				f("grok_com_config.oidc.scopes", "Scopes", TypeStringList, "Default openid profile email offline_access.", nil),
				ph(f("grok_com_config.oidc.audience", "Audience", TypeString, "Required by some IdPs such as Auth0.", nil), "https://api.acme.com"),
			},
		},
		{
			ID: "endpoints", Group: "auth", Title: "Endpoints",
			Fields: []Field{
				ph(f("endpoints.models_base_url", "Models base URL", TypeString, "OpenAI-compatible /v1 root. Switches Grok to API-key auth.", nil), "https://api.acme.com/v1"),
				ph(f("endpoints.models_list_url", "Models list URL", TypeString, "Override when the catalog is not {base_url}/models.", nil), "https://api.acme.com/v1/models"),
			},
		},
		{
			ID: "memory", Group: "memory", Title: "Memory",
			Description: "Experimental. Also enabled by --experimental-memory or GROK_MEMORY=1.",
			Fields: []Field{
				f("memory.enabled", "Enable memory", TypeBool, "Persist and search knowledge across sessions.", false),
				f("memory.session.save_on_end", "Save on session end", TypeBool, "Write a metadata summary when a session ends.", true),
				f("memory.watcher.enabled", "Watch memory files", TypeBool, "Reindex when files under ~/.grok/memory/ change.", true),
				f("memory.search.max_results", "Search max results", TypeInt, "Default number of memory hits.", 6),
				rng(f("memory.search.min_score", "Search min score", TypeFloat, "Minimum relevance for explicit search.", 0.35), 0, 1),
				f("memory.initial_injection.enabled", "First-turn injection", TypeBool, "Auto-inject memory on the first turn.", true),
				rng(f("memory.initial_injection.min_score", "Injection min score", TypeFloat, "0.0 preserves historical no-filter behavior.", 0.0), 0, 1),
				ph(f("memory.embedding.model", "Embedding model", TypeString, "Unset disables vector embeddings.", nil), "embedding-model"),
				f("memory.embedding.dimensions", "Embedding dimensions", TypeInt, "Vector size for the embedding model.", 1024),
			},
		},
		{
			ID: "subagents", Group: "extras", Title: "Subagents",
			Fields: []Field{
				f("subagents.enabled", "Enable subagents", TypeBool, "Allow parallel child sessions. Also GROK_SUBAGENTS.", true),
				f("subagents.toggle", "Type toggles", TypeMap, "Enable or disable named types, e.g. explore = true / plan = false.", nil),
				f("subagents.models", "Type models", TypeMap, "Pin a model per subagent type, e.g. explore = grok-build.", nil),
			},
		},
		{
			ID: "workflows", Group: "extras", Title: "Workflows",
			Fields: []Field{
				f("workflows.enabled", "Background workflows", TypeBool, "workflow tool, /goal host driver, and /workflow launches. Also GROK_WORKFLOWS.", true),
			},
		},
		{
			ID: "skills", Group: "extras", Title: "Skills",
			Fields: []Field{
				f("skills.paths", "Extra skill paths", TypeStringList, "Additional directories to scan.", nil),
				f("skills.ignore", "Ignore paths", TypeStringList, "Skill directories to skip.", nil),
				f("skills.disabled", "Disabled skills", TypeStringList, "Keep listed but inactive.", nil),
			},
		},
		{
			ID: "plugins", Group: "extras", Title: "Plugins",
			Fields: []Field{
				f("plugins.paths", "Extra plugin paths", TypeStringList, "Additional plugin directories.", nil),
				f("plugins.disabled", "Disabled plugins", TypeStringList, "Plugin IDs to skip.", nil),
			},
		},
		{
			ID: "cli", Group: "advanced", Title: "CLI",
			Fields: []Field{
				f("cli.auto_update", "Auto update", TypeBool, "Check for updates on launch.", true),
				ph(f("cli.minimum_version", "Minimum version", TypeString, "Soft anti-downgrade floor for the updater.", nil), "0.2.109"),
				ph(f("cli.maximum_version", "Maximum version", TypeString, "Soft ceiling for the updater.", nil), "0.2.180"),
				ph(f("cli.required_minimum_version", "Required minimum", TypeString, "Refuse to start below this version.", nil), "0.2.100"),
				ph(f("cli.required_maximum_version", "Required maximum", TypeString, "Refuse to start above this version.", nil), "0.2.200"),
			},
		},
		{
			ID: "telemetry", Group: "advanced", Title: "Telemetry",
			Description: "Leave collector URLs empty to use built-in defaults. External OTEL does not require the product-analytics toggle.",
			Fields: []Field{
				ph(f("telemetry.events_url", "Events URL", TypeString, "Send product events to your own collector.", nil), "https://telemetry.example.com/events"),
				secret(f("telemetry.events_api_key", "Events API key", TypeString, "Auth for your events collector.", nil)),
				f("telemetry.mixpanel_enabled", "Mixpanel", TypeBool, "Disable Mixpanel product analytics.", false),
				f("telemetry.trace_upload", "Trace upload", TypeBool, "Session/trace uploads. Follows the telemetry toggle when unset.", nil),
				f("telemetry.otel_enabled", "External OTEL", TypeBool, "Ship a content-free usage schema to your OTLP collector.", false),
				f("telemetry.otel_metrics_exporter", "OTEL metrics exporter", TypeEnum, "", "none", "otlp", "console", "none"),
				f("telemetry.otel_logs_exporter", "OTEL logs exporter", TypeEnum, "", "none", "otlp", "console", "none"),
				ph(f("telemetry.otel_endpoint", "OTEL endpoint", TypeString, "OTLP base endpoint.", nil), "https://collector.example:4318"),
				f("telemetry.otel_protocol", "OTEL protocol", TypeEnum, "", "http/protobuf", "http/protobuf", "grpc"),
				f("telemetry.otel_log_user_prompts", "Log user prompts", TypeBool, "Content gate. Admins can pin this in requirements.toml.", false),
				f("telemetry.otel_log_tool_details", "Log tool details", TypeBool, "Content gate. Admins can pin this in requirements.toml.", false),
			},
		},
		{
			ID: "hints", Group: "advanced", Title: "Hints",
			Description: "Small persisted UI opt-outs. Deleting a key restores the default.",
			Fields: []Field{
				f("hints.project_picker_disabled", "Skip project picker", TypeBool, "Skip the first-prompt directory picker outside a project.", false),
				f("hints.memory_modal_fullscreen", "Memory modal fullscreen", TypeBool, "Remember last memory-modal fullscreen state.", false),
				f("hints.new_session_worktree_mode", "/new worktree prompt", TypeEnum, "", "never", "ask", "always", "never"),
				f("hints.fork_worktree_mode", "/fork worktree prompt", TypeEnum, "", "ask", "ask", "always", "never"),
			},
		},
		{
			ID: "marketplace", Group: "advanced", Title: "Marketplace",
			Fields: []Field{
				f("marketplace.default_skills_installs_purged", "Default skills installs purged", TypeBool, "Internal marketplace flag written by Grok after a one-time cleanup.", nil),
			},
		},
		{
			ID: "compat", Group: "advanced", Title: "Harness compatibility",
			Description: "Each cell defaults to on. Env vars win over config.toml.",
			Fields: []Field{
				f("compat.cursor.skills", "Cursor skills", TypeBool, "Scan ~/.cursor/skills/ and project .cursor/skills/.", true),
				f("compat.cursor.rules", "Cursor rules", TypeBool, "Scan ~/.cursor/rules/ and project rules.", true),
				f("compat.cursor.agents", "Cursor agents", TypeBool, "Scan ~/.cursor/ for named instruction files.", true),
				f("compat.cursor.mcps", "Cursor MCPs", TypeBool, "Scan ~/.cursor/mcp.json and project mcp.json.", true),
				f("compat.cursor.hooks", "Cursor hooks", TypeBool, "Scan ~/.cursor/hooks.json.", true),
				f("compat.cursor.sessions", "Cursor sessions", TypeBool, "Staged; no scanner consumer yet.", true),
				f("compat.claude.skills", "Claude skills", TypeBool, "Scan ~/.claude/skills/ and project .claude/skills/.", true),
				f("compat.claude.rules", "Claude rules", TypeBool, "Scan ~/.claude/rules/.", true),
				f("compat.claude.agents", "Claude agents", TypeBool, "Scan ~/.claude/ and CLAUDE*.md files.", true),
				f("compat.claude.mcps", "Claude MCPs", TypeBool, "Scan ~/.claude.json for MCP servers.", true),
				f("compat.claude.hooks", "Claude hooks", TypeBool, "Scan ~/.claude/settings.json for hooks.", true),
				f("compat.claude.sessions", "Claude sessions", TypeBool, "Staged; no scanner consumer yet.", true),
				f("compat.codex.sessions", "Codex sessions", TypeBool, "Staged; no scanner consumer yet.", true),
			},
		},
		{
			ID: "shell_environment_policy", Group: "advanced", Title: "Shell environment policy",
			Description: "Filters environment variables inherited by tool subprocesses.",
			Fields: []Field{
				f("shell_environment_policy.inherit", "Inherit", TypeEnum, "all keeps everything, core keeps PATH/HOME-like vars, none starts empty.", "all", "all", "core", "none"),
				f("shell_environment_policy.ignore_default_excludes", "Ignore default secret excludes", TypeBool, "When false, also drop *KEY* / *SECRET* / *TOKEN*. Default true (leave env untouched).", true),
				f("shell_environment_policy.exclude", "Exclude", TypeStringList, "Drop matching names. Case-insensitive globs.", nil),
				f("shell_environment_policy.include_only", "Include only", TypeStringList, "If set, keep only these names after other filters.", nil),
				f("shell_environment_policy.set", "Force values", TypeMap, "Force these environment values, e.g. MY_FLAG = 1.", nil),
			},
		},
	}
}

func Collections() []Collection {
	return []Collection{
		{
			ID: "models", Group: "models", Prefix: "model", KeyLabel: "Model ID",
			Title:       "Custom models",
			Description: "Each [model.<id>] is a picker entry. Override a built-in model by using its name as the ID.",
			Fields:      modelFields(),
			Templates: []Template{
				{
					ID: "openai", Label: "OpenAI", SuggestedID: "gpt-4o",
					Description: "OpenAI Chat Completions",
					Values: map[string]any{
						"model":       "gpt-4o",
						"name":        "GPT-4o",
						"base_url":    "https://api.openai.com/v1",
						"api_backend": "chat_completions",
						"env_key":     "OPENAI_API_KEY",
					},
				},
				{
					ID: "openai-responses", Label: "OpenAI (Responses)", SuggestedID: "gpt-4o-responses",
					Description: "OpenAI Responses API",
					Values: map[string]any{
						"model":       "gpt-4o",
						"name":        "GPT-4o (Responses)",
						"base_url":    "https://api.openai.com/v1",
						"api_backend": "responses",
						"env_key":     "OPENAI_API_KEY",
					},
				},
				{
					ID: "anthropic", Label: "Anthropic (Claude)", SuggestedID: "claude-opus",
					Description: "Anthropic Messages API. Key goes in extra_headers, not Bearer.",
					Values: map[string]any{
						"model":          "claude-opus-4-6",
						"name":           "Claude Opus",
						"base_url":       "https://api.anthropic.com/v1",
						"api_backend":    "messages",
						"context_window": int64(200000),
						"extra_headers": map[string]any{
							"anthropic-version": "2023-06-01",
						},
					},
				},
				{
					ID: "ollama", Label: "Ollama (local)", SuggestedID: "ollama-codellama",
					Description: "Local OpenAI-compatible server on port 11434",
					Values: map[string]any{
						"model":    "codellama",
						"name":     "CodeLlama (Ollama)",
						"base_url": "http://localhost:11434/v1",
					},
				},
				{
					ID: "openai-compatible", Label: "OpenAI-compatible", SuggestedID: "custom",
					Description: "Any /v1 Chat Completions endpoint",
					Values: map[string]any{
						"model":       "model-id",
						"name":        "Custom model",
						"base_url":    "https://api.example.com/v1",
						"api_backend": "chat_completions",
					},
				},
				{
					ID: "xai", Label: "xAI / SpaceXAI proxy", SuggestedID: "grok-build",
					Description: "Override or proxy a Grok model",
					Values: map[string]any{
						"model":                     "grok-4.6",
						"name":                      "Grok",
						"base_url":                  "https://api.x.ai/v1",
						"api_backend":               "chat_completions",
						"env_key":                   "XAI_API_KEY",
						"supports_reasoning_effort": true,
					},
				},
			},
		},
		{
			ID: "providers", Group: "models", Prefix: "model_providers", KeyLabel: "Provider ID",
			Title:       "Model providers",
			Description: "Shared connection settings. Point a model at one with model_provider = \"id\".",
			Fields:      providerFields(),
			Templates: []Template{
				{
					ID: "openai", Label: "OpenAI", SuggestedID: "openai",
					Values: map[string]any{
						"name":         "OpenAI",
						"api_base_url": "https://api.openai.com/v1",
						"api_backend":  "chat_completions",
						"env_key":      "OPENAI_API_KEY",
					},
				},
				{
					ID: "anthropic", Label: "Anthropic", SuggestedID: "anthropic",
					Values: map[string]any{
						"name":         "Anthropic",
						"api_base_url": "https://api.anthropic.com/v1",
						"api_backend":  "messages",
						"env_key":      "ANTHROPIC_API_KEY",
						"extra_headers": map[string]any{
							"anthropic-version": "2023-06-01",
						},
					},
				},
				{
					ID: "ollama", Label: "Ollama", SuggestedID: "ollama",
					Values: map[string]any{
						"name":         "Ollama",
						"api_base_url": "http://localhost:11434/v1",
						"api_backend":  "chat_completions",
					},
				},
				{
					ID: "custom", Label: "Custom gateway", SuggestedID: "gateway",
					Values: map[string]any{
						"name":         "Gateway",
						"api_base_url": "https://gateway.example/v1",
						"api_backend":  "chat_completions",
					},
				},
			},
		},
		{
			ID: "mcp_servers", Group: "mcp", Prefix: "mcp_servers", KeyLabel: "Server name",
			Title:       "MCP servers",
			Description: "stdio (command + args) or remote (url). Names: letters, numbers, hyphens, underscores.",
			Fields:      mcpFields(),
			Templates: []Template{
				{
					ID: "filesystem", Label: "Filesystem", SuggestedID: "filesystem",
					Values: map[string]any{
						"command": "npx",
						"args":    []any{"-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"},
						"enabled": true,
					},
				},
				{
					ID: "github", Label: "GitHub", SuggestedID: "github",
					Values: map[string]any{
						"command": "npx",
						"args":    []any{"-y", "@modelcontextprotocol/server-github"},
						"enabled": true,
					},
				},
				{
					ID: "http", Label: "Remote HTTP", SuggestedID: "remote",
					Values: map[string]any{
						"url":     "https://mcp.example.com/mcp",
						"enabled": true,
					},
				},
			},
		},
	}
}

func fieldIndex() map[string]Field {
	out := map[string]Field{}
	for _, sec := range Sections() {
		for _, field := range sec.Fields {
			out[field.Key] = field
		}
	}
	return out
}

func collectionByID(id string) (Collection, bool) {
	for _, c := range Collections() {
		if c.ID == id {
			return c, true
		}
	}
	return Collection{}, false
}

func collectionField(col Collection, key string) (Field, bool) {
	for _, field := range col.Fields {
		if field.Key == key {
			return field, true
		}
	}
	return Field{}, false
}
