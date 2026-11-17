#!/bin/bash
# Notification settings can be imported by specifying the member's user ID. The
# rules themselves come from configuration, so the first apply after the import
# writes them; the write is idempotent and changes nothing that already matches.
terraform import clickhouse_notification_settings.example xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
