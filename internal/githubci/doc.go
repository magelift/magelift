// Package githubci provides a bounded HTTP client for GitHub repository
// metadata and non-secret MAGELIFT_ Actions variables used during account
// preparation. Callers supply the API token; this package does not resolve
// credentials from the environment or invoke the gh CLI.
package githubci
