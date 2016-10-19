import * as PatternUtils from "src/pattern-utils";

// urlToDefInfo returns a URL to the given def's info at the given revision.
export function urlToDefInfo(def: any, rev?: string | null): string {
	if ((def.File === null || def.Kind === "package")) {
		// The def's File field refers to a directory (e.g., in the
		// case of a Go package). We can't show a dir in this view,
		// so just redirect to the dir listing.
		//
		// TODO(sqs): Improve handling of this case.
		let file = def.File === "." ? "" : def.File;
		return urlToTree(def.Repo || "", rev || null, file);
	}
	return `${urlToRepoRev(def.Repo || "", rev || "")}/-/info/${def.UnitType}/${def.Unit}/-/${def.Path}`;
}

// urlToTree generates the URL to a dir. To get a file's URL, use urlToBlob.
export function urlToTree(repo: string, rev: string | null, path: string | string[]): string {
	rev = rev || "";

	// Fast-path: we redirect the tree root to the repo route anyway, so just construct
	// the repo route URL directly.
	if (!path || path === "/" || path.length === 0) {
		return urlToRepoRev(repo, rev);
	}

	const pathStr = typeof path === "string" ? path : path.join("/");
	return urlTo("tree", {splat: [makeRepoRev(repo, rev), pathStr]} as any);
}

export function urlToRepoRev(repo: string, rev: string | null): string {
	return urlTo("repo", { splat: makeRepoRev(repo, rev) });
}

// urlTo produces the full URL, given a route and route parameters. The
// route names are defined in sourcegraph/app/routePatterns.
export function urlTo(name: string, params: {}): string {
	return PatternUtils.formatPattern(`/${getAbs()[name]}`, params);
}

// makeRepoRev returns "<repo>@<rev>" if rev is a non-empty string, otherwise
// it returns just "<repo>".
export function makeRepoRev(repo: string, rev: string | null): string {
	if (rev) { return `${repo}@${rev}`; }
	return repo;
}

export function getRel() {
	return {
		// NOTE: If you add a top-level route (e.g., "/about"), add it to the
		// topLevel list in app/internal/ui/router.go.
		about: "about",
		beta: "beta",
		docs: "docs",
		contact: "contact",
		security: "security",
		pricing: "pricing",
		terms: "terms",
		privacy: "privacy",
		styleguide: "styleguide",
		integrations: "integrations",
		settings: "settings",
		login: "login",
		signup: "join",

		home: "",

		repo: "*", // matches both "repo" and "repo@rev"
		commit: "commit",
		tree: "tree/*",
		blob: "blob/*",
	};
}

export function getAbs() {
	const rel = getRel();
	return {
	about: rel.about,
	contact: rel.contact,
	security: rel.security,
	pricing: rel.pricing,
	terms: rel.terms,
	privacy: rel.privacy,
	styleguide: rel.styleguide,
	home: rel.home,
	integrations: rel.integrations,
	settings: rel.settings,
	login: rel.login,
	signup: rel.signup,

	repo: rel.repo,
	commit: `${rel.repo}/-/${rel.commit}`,
	tree: `${rel.repo}/-/${rel.tree}`,
	blob: `${rel.repo}/-/${rel.blob}`,
};
}