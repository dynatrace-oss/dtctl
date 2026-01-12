#!/bin/bash

set -e

SPECS_DIR="api-spec"
OUTPUT_DIR="pkg/api"

echo "Generating API clients from OpenAPI specifications..."

# Check if oapi-codegen is installed
if ! command -v oapi-codegen &> /dev/null; then
    echo "oapi-codegen not found. Installing..."
    go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest
fi

mkdir -p "$OUTPUT_DIR"

# Generate Automation API client (workflows)
if [ -f "$SPECS_DIR/automation.yaml" ]; then
    echo "Generating automation client..."
    mkdir -p "$OUTPUT_DIR/automation"
    oapi-codegen -generate types,client \
      -package automation \
      "$SPECS_DIR/automation.yaml" > "$OUTPUT_DIR/automation/client.go"
fi

# Generate Grail Query API client (DQL)
if [ -f "$SPECS_DIR/grail-query.yaml" ]; then
    echo "Generating grail-query client..."
    mkdir -p "$OUTPUT_DIR/grail"
    oapi-codegen -generate types,client \
      -package grail \
      "$SPECS_DIR/grail-query.yaml" > "$OUTPUT_DIR/grail/client.go"
fi

# Generate Document API client
if [ -f "$SPECS_DIR/document.yaml" ]; then
    echo "Generating document client..."
    mkdir -p "$OUTPUT_DIR/document"
    oapi-codegen -generate types,client \
      -package document \
      "$SPECS_DIR/document.yaml" > "$OUTPUT_DIR/document/client.go"
fi

# Generate SLO API client
if [ -f "$SPECS_DIR/slo.yaml" ]; then
    echo "Generating slo client..."
    mkdir -p "$OUTPUT_DIR/slo"
    oapi-codegen -generate types,client \
      -package slo \
      "$SPECS_DIR/slo.yaml" > "$OUTPUT_DIR/slo/client.go"
fi

# Generate Settings API client
if [ -f "$SPECS_DIR/settings.yaml" ]; then
    echo "Generating settings client..."
    mkdir -p "$OUTPUT_DIR/settings"
    oapi-codegen -generate types,client \
      -package settings \
      "$SPECS_DIR/settings.yaml" > "$OUTPUT_DIR/settings/client.go"
fi

echo "API client generation complete!"
echo "Generated clients in $OUTPUT_DIR/"
