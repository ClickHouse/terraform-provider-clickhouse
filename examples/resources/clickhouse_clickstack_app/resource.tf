# An app whose TSX source lives next to the configuration as a plain App.tsx.
# Apps can't take parameters, so values such as source ids go into the code:
# App.tsx holds a placeholder string, and replace() swaps in the real id. The
# file stays valid TSX that editors and linters understand.
resource "clickhouse_clickstack_app" "checkout_errors" {
  name    = "Checkout errors"
  purpose = "Shows checkout errors by service for on-call engineers."
  code = replace(
    file("${path.module}/App.tsx"),
    "__SOURCE_ID__",
    clickhouse_clickstack_source.logs.id,
  )

  # Defaults to true. A private app is visible only to the API key's user.
  is_shared = true
}
