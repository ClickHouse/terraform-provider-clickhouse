# An app whose TSX source lives next to the configuration as a plain App.tsx.
# Apps can't take parameters, so values such as source ids go into the code:
# App.tsx holds one '__CONFIG__' placeholder, and replace() swaps in every value
# at once as a jsonencode()d object. jsonencode() quotes and escapes strings
# (so a value like "Bob's errors" is safe) and keeps numbers and booleans as
# they are. The file stays valid TSX that editors and linters understand.
resource "clickhouse_clickstack_app" "checkout_errors" {
  name    = "Checkout errors"
  purpose = "Shows checkout errors by service for on-call engineers."
  code = replace(file("${path.module}/App.tsx"), "'__CONFIG__'", jsonencode({
    sourceId = clickhouse_clickstack_source.logs.id
    heading  = "Bob's checkout errors"
  }))

  # Defaults to true. A private app is visible only to the API key's user.
  is_shared = true
}
