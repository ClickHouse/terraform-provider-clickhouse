resource "clickhouse_service" "service" {
  ...
}

resource "aws_kms_key" "enc" {
  ...
}

resource "clickhouse_service_transparent_data_encryption_key_association" "service_key_association" {
  service_id = clickhouse_service.service.id
  key_id = aws_kms_key.enc.arn
}
