const PLUGIN_SETTINGS_KEY = "plugin.inlang.messageFormat";
const MESSAGE_SCHEMA = "https://inlang.com/schema/inlang-message-format";

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

const plugin = {
	key: PLUGIN_SETTINGS_KEY,
	toBeImportedFiles: async ({ settings }) => {
		const patterns = getPathPatterns(settings);
		return settings.locales.flatMap((locale) =>
			patterns.map((pattern) => ({
				locale,
				path: pattern.replaceAll("{locale}", locale).replaceAll("{languageTag}", locale)
			}))
		);
	},
	importFiles: async ({ files }) => {
		const bundleVariables = new Map();
		const messages = [];
		const variants = [];

		for (const file of files) {
			const parsed = JSON.parse(textDecoder.decode(file.content));
			for (const [bundleId, value] of flattenMessages(parsed)) {
				const { pattern, variables } = parsePattern(value);
				const variableSet = bundleVariables.get(bundleId) ?? new Set();
				for (const variable of variables) {
					variableSet.add(variable);
				}
				bundleVariables.set(bundleId, variableSet);
				messages.push({
					bundleId,
					locale: file.locale,
					selectors: []
				});
				variants.push({
					messageBundleId: bundleId,
					messageLocale: file.locale,
					matches: [],
					pattern
				});
			}
		}

		const bundles = [...bundleVariables.entries()].map(([id, variables]) => ({
			id,
			declarations: [...variables].map((name) => ({ type: "input-variable", name }))
		}));

		return { bundles, messages, variants };
	},
	exportFiles: async ({ settings, messages, variants }) => {
		const [pathPattern] = getPathPatterns(settings);
		return settings.locales.map((locale) => {
			const json = { $schema: MESSAGE_SCHEMA };
			for (const message of messages.filter((candidate) => candidate.locale === locale)) {
				const variant = variants.find((candidate) => candidate.messageId === message.id);
				if (variant) {
					json[message.bundleId] = serializePattern(variant.pattern);
				}
			}
			return {
				locale,
				path: pathPattern.replaceAll("{locale}", locale).replaceAll("{languageTag}", locale),
				content: textEncoder.encode(JSON.stringify(json, null, 2) + "\n")
			};
		});
	}
};

function getPathPatterns(settings) {
	const pathPattern = settings[PLUGIN_SETTINGS_KEY]?.pathPattern;
	if (Array.isArray(pathPattern)) {
		return pathPattern;
	}
	return [pathPattern ?? "./messages/{locale}.json"];
}

function flattenMessages(value, prefix = "") {
	const messages = [];
	for (const [key, nestedValue] of Object.entries(value)) {
		if (key === "$schema") {
			continue;
		}
		const messageKey = prefix ? `${prefix}.${key}` : key;
		if (typeof nestedValue === "string") {
			messages.push([messageKey, nestedValue]);
		} else if (nestedValue && typeof nestedValue === "object" && Array.isArray(nestedValue) === false) {
			messages.push(...flattenMessages(nestedValue, messageKey));
		} else {
			throw new Error(`Unsupported message value for "${messageKey}". Expected a string or nested object.`);
		}
	}
	return messages;
}

function parsePattern(message) {
	const pattern = [];
	const variables = [];
	const variablePattern = /\{([A-Za-z_$][\w$]*)\}/g;
	let lastIndex = 0;
	for (const match of message.matchAll(variablePattern)) {
		if (match.index > lastIndex) {
			pattern.push({ type: "text", value: message.slice(lastIndex, match.index) });
		}
		const name = match[1];
		variables.push(name);
		pattern.push({
			type: "expression",
			arg: { type: "variable-reference", name }
		});
		lastIndex = match.index + match[0].length;
	}
	if (lastIndex < message.length) {
		pattern.push({ type: "text", value: message.slice(lastIndex) });
	}
	return { pattern, variables };
}

function serializePattern(pattern) {
	return pattern
		.map((part) => {
			if (part.type === "text") {
				return part.value;
			}
			if (part.type === "expression" && part.arg?.type === "variable-reference") {
				return `{${part.arg.name}}`;
			}
			throw new Error(`Unsupported pattern part type "${part.type}".`);
		})
		.join("");
}

export default plugin;
