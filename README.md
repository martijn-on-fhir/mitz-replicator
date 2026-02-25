# Mitz Replicator

A lightweight Go (Gin) HTTPS server that mimics the VZVZ Mitz consent register for local testing of `mitz-connector`.

## Endpoints

| Method | Path     | Purpose                                      |
|--------|----------|----------------------------------------------|
| HEAD   | `/xacml` | Health-check probe (mTLS connectivity test)  |
| POST   | `/xacml` | Gesloten autorisatievraag (XACML 3.0 / 2.0) |
| POST   | `/xcpd`  | Open autorisatievraag (XCPD + XUA SAML)      |

All POST endpoints accept and return `Content-Type: application/soap+xml; charset=utf-8`.

## Quick Start

### 1. Generate certificates

```bash
cd certs
bash generate.sh
cd ..
```

This creates a self-signed CA, server certificate (CN=localhost with SAN), and client certificate (CN=mitz-connector).

### 2. Run the server

```bash
go run main.go
```

The server starts on `https://localhost:8443` by default.

### 3. Test connectivity

```bash
# Health check
curl -k -I https://localhost:8443/xacml

# Health check with mTLS client cert
curl --cert certs/client.crt --key certs/client.key --cacert certs/ca.crt -I https://localhost:8443/xacml
```

## Configuration

Configuration is done via environment variables:

| Variable      | Default            | Description                        |
|---------------|--------------------|------------------------------------|
| `PORT`        | `8443`             | Listen port                        |
| `SERVER_CERT` | `certs/server.crt` | Server certificate path            |
| `SERVER_KEY`  | `certs/server.key` | Server private key path            |
| `CA_CERT`     | `certs/ca.crt`     | CA certificate for client verification |
| `MTLS_ENABLED`| `false`            | Require and verify client certificates |

Example with mTLS enabled:

```bash
MTLS_ENABLED=true go run main.go
```

## BSN-Based Mock Routing

The server routes responses based on the patient BSN extracted from the request:

| BSN         | `/xacml` Response              | `/xcpd` Response                         |
|-------------|--------------------------------|------------------------------------------|
| `000000001` | All Permit                     | 2 locations with multiple event codes    |
| `000000002` | All Deny                       | 1 location with 1 event code             |
| `000000003` | First Permit, rest Deny        | Empty response (patient not found)       |
| `000000004` | All Indeterminate              | SOAP Fault                               |
| `000000005` | SOAP Fault                     | SOAP Fault                               |
| `999*` / default | All Permit                | 1 location with huisarts + medicatie     |

## Configuring mitz-connector

Point the connector at this mock server:

```env
MITZ_ENDPOINT=https://localhost:8443/xacml
MITZ_OPEN_ENDPOINT=https://localhost:8443/xcpd
MITZ_CERT_PATH=./certs/client.crt
MITZ_KEY_PATH=./certs/client.key
MITZ_CA_PATH=./certs/ca.crt
```

## Project Structure

```
mitz-replicator/
├── main.go              # Gin server, TLS config, template loading
├── handlers/
│   ├── health.go        # HEAD /xacml
│   ├── xacml.go         # POST /xacml with BSN routing
│   └── xcpd.go          # POST /xcpd with BSN routing
├── parser/
│   └── request.go       # XACML + XCPD request parsing
├── templates/
│   ├── xacml_response.xml
│   ├── xacml_fault.xml
│   ├── xcpd_found.xml
│   ├── xcpd_empty.xml
│   └── xcpd_fault.xml
├── certs/
│   ├── generate.sh      # Certificate generation script
│   └── .gitignore
├── go.mod
└── go.sum
```

## Example Requests

### XACML — Gesloten autorisatievraag

```bash
curl -sk -X POST https://localhost:8443/xacml \
  -H "Content-Type: application/soap+xml; charset=utf-8" \
  -H "X-Request-Id: test-001" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <xacml-samlp:XACMLAuthzDecisionQuery
        xmlns:xacml-samlp="urn:oasis:names:tc:xacml:3.0:profile:saml2.0:v2:schema:protocol"
        xmlns:xacml-context="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
      <xacml-context:Request>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource">
          <xacml-context:Attribute AttributeId="urn:oasis:names:tc:xacml:2.0:resource:resource-id">
            <xacml-context:AttributeValue>000000001</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code">
            <xacml-context:AttributeValue>2.16.840.1.113883.2.4.3.111.5.10.1^huisartsgegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code">
            <xacml-context:AttributeValue>2.16.840.1.113883.2.4.3.111.5.10.1^medicatiegegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
      </xacml-context:Request>
    </xacml-samlp:XACMLAuthzDecisionQuery>
  </soap:Body>
</soap:Envelope>'
```

### XCPD — Open autorisatievraag

```bash
curl -sk -X POST https://localhost:8443/xcpd \
  -H "Content-Type: application/soap+xml; charset=utf-8" \
  -H "X-Request-Id: test-002" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <PRPA_IN201305UV02 xmlns="urn:hl7-org:v3">
      <sender typeCode="SND">
        <device classCode="DEV" determinerCode="INSTANCE">
          <id root="00005678"/>
        </device>
      </sender>
      <controlActProcess classCode="CACT" moodCode="EVN">
        <queryByParameter>
          <parameterList>
            <livingSubjectId>
              <value root="2.16.840.1.113883.2.4.6.3" extension="000000001"/>
            </livingSubjectId>
          </parameterList>
        </queryByParameter>
      </controlActProcess>
    </PRPA_IN201305UV02>
  </soap:Body>
</soap:Envelope>'
```
