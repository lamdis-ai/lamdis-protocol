package agent

// Catalog is the list of services Lamdis has checked by hand: the vendor's
// own MCP address and how signing in works there. Probed 2026-09-24.
//
// "oauth" entries whose note says a registered app is needed only connect
// once this host has one for that vendor (LAMDIS_OAUTH_CLIENTS); until then
// the Connect card says so rather than failing somewhere confusing.
// "none" entries are here so the agent can say plainly that there is no
// sanctioned way in, instead of guessing or suggesting a scraper.
var Catalog = []CatalogEntry{
	// Connect directly: the service lets a new app introduce itself.
	{Name: "Notion", URL: "https://mcp.notion.com/mcp", How: "oauth"},
	{Name: "Linear", URL: "https://mcp.linear.app/mcp", How: "oauth"},
	{Name: "Atlassian", Aliases: []string{"jira", "confluence"}, URL: "https://mcp.atlassian.com/v1/mcp", How: "oauth", Note: "Jira and Confluence."},
	{Name: "Stripe", URL: "https://mcp.stripe.com", How: "oauth"},
	{Name: "Canva", URL: "https://mcp.canva.com/mcp", How: "oauth"},
	{Name: "Airtable", URL: "https://mcp.airtable.com/mcp", How: "oauth"},
	{Name: "Todoist", URL: "https://ai.todoist.net/mcp", How: "oauth"},
	{Name: "Dropbox", URL: "https://mcp.dropbox.com/mcp", How: "oauth"},
	{Name: "Intercom", URL: "https://mcp.intercom.com/mcp", How: "oauth"},
	{Name: "Sentry", URL: "https://mcp.sentry.dev/mcp", How: "oauth"},
	{Name: "Cloudflare", URL: "https://mcp.cloudflare.com/mcp", How: "oauth"},
	{Name: "Vercel", URL: "https://mcp.vercel.com", How: "oauth"},
	{Name: "PayPal", URL: "https://mcp.paypal.com/mcp", How: "oauth"},
	{Name: "Square", URL: "https://mcp.squareup.com/sse", How: "oauth"},
	{Name: "monday.com", Aliases: []string{"monday"}, URL: "https://mcp.monday.com/mcp", How: "oauth"},
	{Name: "ClickUp", URL: "https://mcp.clickup.com/mcp", How: "oauth"},
	{Name: "Figma", URL: "https://mcp.figma.com/mcp", How: "oauth", Note: "Figma may limit which apps it lets in."},
	{Name: "Meta Ads", Aliases: []string{"facebook ads", "instagram ads", "meta ads manager"}, URL: "https://mcp.facebook.com/ads", How: "oauth", Note: "Ads only: campaigns, budgets and results."},
	{Name: "Zapier", URL: "https://mcp.zapier.com/api/mcp/mcp", How: "oauth", Note: "Reaches thousands of other apps through the person's own Zapier account; each action uses their Zapier tasks."},
	{Name: "DeepWiki", URL: "https://mcp.deepwiki.com/mcp", How: "open", Note: "Reads public GitHub repositories' documentation."},

	// Official, but the vendor only admits apps it has approved.
	{Name: "Gmail", Aliases: []string{"google mail", "email"}, URL: "https://gmailmcp.googleapis.com/mcp/v1", How: "oauth", Note: "Needs a Google-approved Lamdis app, which is not in place yet."},
	{Name: "Google Calendar", Aliases: []string{"calendar", "gcal"}, URL: "https://calendarmcp.googleapis.com/mcp/v1", How: "oauth", Note: "Needs a Google-approved Lamdis app, which is not in place yet."},
	{Name: "Google Drive", Aliases: []string{"drive", "google docs", "google sheets"}, URL: "https://drivemcp.googleapis.com/mcp/v1", How: "oauth", Note: "Needs a Google-approved Lamdis app, which is not in place yet."},
	{Name: "GitHub", URL: "https://api.githubcopilot.com/mcp/", How: "oauth", Note: "Needs a registered GitHub app for Lamdis; a personal access token also works."},
	{Name: "Slack", URL: "https://mcp.slack.com/mcp", How: "oauth", Note: "Needs a registered Slack app for Lamdis, which is not in place yet."},
	{Name: "HubSpot", URL: "https://mcp.hubspot.com", How: "oauth", Note: "Needs a HubSpot app for Lamdis, which is not in place yet."},
	{Name: "Box", URL: "https://mcp.box.com", How: "oauth", Note: "Needs a Box app for Lamdis, which is not in place yet."},
	{Name: "Asana", URL: "https://mcp.asana.com/v2/mcp", How: "oauth", Note: "Needs an Asana app for Lamdis, which is not in place yet."},

	// No sanctioned way in. Say so; do not suggest password-sharing tools.
	{Name: "Blink", Aliases: []string{"blink camera", "blink cameras"}, How: "none", Note: "Amazon offers no public API for Blink. If they run Home Assistant with its Blink integration, connecting Home Assistant is the supported route."},
	{Name: "Ring", Aliases: []string{"ring doorbell", "ring camera"}, How: "none", Note: "No public API. Home Assistant's Ring integration is the supported route if they run it."},
	{Name: "Expedia", How: "none", Note: "Expedia's agent tools are for business partners only; a personal Expedia account cannot be connected."},
	{Name: "Facebook", Aliases: []string{"facebook account", "facebook page", "instagram", "meta"}, How: "none", Note: "Meta does not let apps read or post for a personal account. Ads can be connected (Meta Ads); Pages and Instagram posting need an approved Meta app, or the person's own Zapier."},
	{Name: "Airbnb", How: "none", Note: "Partner-only API."},
	{Name: "Booking.com", Aliases: []string{"booking"}, How: "none", Note: "Partner-only API."},
	{Name: "Uber", How: "none", Note: "Uber's agent access is partner-only."},
	{Name: "Spotify", How: "none", Note: "No official MCP server, and new Spotify apps are limited to a few test users."},
	{Name: "Philips Hue", Aliases: []string{"hue"}, How: "none", Note: "No hosted API for agents; Home Assistant's Hue integration is the supported route."},
	{Name: "Shopify", How: "none", Note: "No hosted admin connection; each store's storefront has its own at https://<store>.myshopify.com/api/mcp."},
}
