#!/bin/bash

# ============================================
# Setup Debezium CDC Connector
# ============================================
# Run this after Kafka and Debezium are running
# ./scripts/setup-debezium.sh

set -e

DEBEZIUM_URL="http://localhost:8083"

echo "🔄 Setting up Debezium PostgreSQL Connector..."

# Wait for Debezium to be ready
echo "⏳ Waiting for Debezium Connect to be ready..."
until curl -s "$DEBEZIUM_URL/connectors" > /dev/null 2>&1; do
    echo "   Debezium not ready yet, waiting..."
    sleep 5
done
echo "✓ Debezium Connect is ready"

# Check if connector already exists
if curl -s "$DEBEZIUM_URL/connectors/risk-engine-connector" > /dev/null 2>&1; then
    echo "⚠️  Connector already exists. Deleting..."
    curl -s -X DELETE "$DEBEZIUM_URL/connectors/risk-engine-connector"
    sleep 2
fi

# Create the connector
echo "📝 Creating PostgreSQL CDC connector..."

curl -s -X POST "$DEBEZIUM_URL/connectors" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "risk-engine-connector",
        "config": {
            "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
            "database.hostname": "postgres",
            "database.port": "5432",
            "database.user": "postgres",
            "database.password": "postgres",
            "database.dbname": "risk_engine",
            "database.server.name": "risk-engine",
            "topic.prefix": "risk-engine",
            "table.include.list": "public.transactions",
            "plugin.name": "pgoutput",
            "publication.autocreate.mode": "filtered",
            "slot.name": "risk_engine_slot",
            "heartbeat.interval.ms": "10000",
            "snapshot.mode": "initial",
            "transforms": "unwrap",
            "transforms.unwrap.type": "io.debezium.transforms.ExtractNewRecordState",
            "transforms.unwrap.drop.tombstones": "true",
            "transforms.unwrap.delete.handling.mode": "rewrite",
            "key.converter": "org.apache.kafka.connect.json.JsonConverter",
            "key.converter.schemas.enable": "false",
            "value.converter": "org.apache.kafka.connect.json.JsonConverter",
            "value.converter.schemas.enable": "false"
        }
    }' | jq '.'

echo ""
echo "✓ Connector created!"
echo ""

# Verify connector status
echo "📊 Connector Status:"
curl -s "$DEBEZIUM_URL/connectors/risk-engine-connector/status" | jq '.'

echo ""
echo "🎉 Debezium CDC is now capturing changes from PostgreSQL!"
echo ""
echo "Topics created:"
echo "  - risk-engine.public.transactions (transaction changes)"
echo ""
echo "View in Kafka UI: http://localhost:8090"
