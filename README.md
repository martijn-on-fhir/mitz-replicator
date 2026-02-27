# Mitz Replicator

A lightweight Go (Gin) HTTPS server that mimics the VZVZ Mitz consent register for local testing of `mitz-connector`.

## Endpoints

### SOAP Endpoints

| Method | Path     | Purpose                                      |
|--------|----------|----------------------------------------------|
| HEAD   | `/xacml` | Health-check probe (mTLS connectivity test)  |
| POST   | `/xacml` | Gesloten autorisatievraag (XACML 3.0 / 2.0) |
| POST   | `/xcpd`  | Open autorisatievraag (XCPD + XUA SAML)      |

SOAP endpoints accept and return `Content-Type: application/soap+xml; charset=utf-8`.

### FHIR Endpoints

| Method | Path                                     | Purpose                                      |
|--------|------------------------------------------|----------------------------------------------|
| POST   | `/fhir/Subscription`                     | Create consent subscription (OTV-TR-0120)    |
| DELETE | `/fhir/Subscription/:id`                 | Cancel subscription (OTV-TR-0130)            |
| POST   | `/fhir/`                                 | Bundle transaction — migration (OTV-TR-0150) or toestemmingsknop (OTV-TR-0160) |
| GET    | `/fhir/Subscription/$processingStatus`   | Query Subscription processing status         |
| GET    | `/fhir/Consent/$processingStatus`        | Query Consent processing status              |

FHIR endpoints accept and return `Content-Type: application/fhir+xml; charset=utf-8`.

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

### Test connectivity

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

## SAML Assertion Validation

The replicator can validate `Authorization: SAML <base64>` headers sent by the connector on FHIR endpoints, catching bugs in the connector's SAML implementation during local testing.

### Configuration

| Variable | Default | Description |
|---|---|---|
| `SAML_VALIDATION_ENABLED` | `false` | Toggle SAML assertion validation on FHIR endpoints |
| `SAML_SIGNING_CERT` | `certs/client.crt` | PEM certificate for XML-DSig signature verification |
| `SAML_EXPECTED_ISSUER` | _(empty = skip)_ | Expected SAML Issuer value (empty skips check) |
| `SAML_CLOCK_SKEW_SECONDS` | `5` | Allowed clock skew in seconds for temporal checks |

### Protected Endpoints

When `SAML_VALIDATION_ENABLED=true`:

| Endpoint | Transaction | Validation |
|---|---|---|
| `POST /fhir/Subscription` | OTV-TR-0120 | Middleware — checked before handler |
| `DELETE /fhir/Subscription/:id` | OTV-TR-0130 | Middleware — checked before handler |
| `POST /fhir/` (migration bundle) | OTV-TR-0150 | Handler-level — checked after body parsing |
| `POST /fhir/` (toestemmingsknop bundle) | OTV-TR-0160 | **Not checked** — uses Bearer JWT |

Migration vs toestemmingsknop bundles are distinguished by the presence of a `Provenance` resource in the bundle entries.

### Validation Checks

1. **XML-DSig signature** — verifies the enveloped signature using the configured PEM certificate via `goxmldsig`
2. **Issuer** — if `SAML_EXPECTED_ISSUER` is set, verifies the `<saml:Issuer>` element matches
3. **Temporal conditions** — checks `<saml:Conditions>` `NotBefore` and `NotOnOrAfter` attributes with clock skew tolerance

Failed validation returns HTTP 401 with a FHIR `OperationOutcome` containing the error details.

### Example

```bash
SAML_VALIDATION_ENABLED=true \
SAML_SIGNING_CERT=C:/private/mitz-connector/certs/certificate.crt \
SAML_EXPECTED_ISSUER=mitz-connector \
go run main.go
```

## BSN-Based Mock Routing

### SOAP Endpoints

The server routes responses based on the patient BSN extracted from the request:

| BSN         | `/xacml` Response              | `/xcpd` Response                         |
|-------------|--------------------------------|------------------------------------------|
| `000000001` | All Permit                     | 2 locations with multiple event codes    |
| `000000002` | All Deny                       | 1 location with 1 event code             |
| `000000003` | First Permit, rest Deny        | Empty response (patient not found)       |
| `000000004` | All Indeterminate              | SOAP Fault                               |
| `000000005` | SOAP Fault                     | SOAP Fault                               |
| `999*` / default | All Permit                | 1 location with huisarts + medicatie     |

### FHIR Endpoints

FHIR endpoints route on BSN (extracted from Subscription criteria or Bundle Patient entry):

| BSN / Provider ID | Subscription (POST)       | Bundle (POST /)           | $processingStatus (GET)     |
|--------------------|---------------------------|---------------------------|-----------------------------|
| `000000001`        | 202 Accepted (GUID)       | 200 OK (transaction-response) | —                       |
| `000000002`        | 202 Accepted (GUID)       | 200 OK (transaction-response) | —                       |
| `000000003`        | 400 OperationOutcome      | 400 OperationOutcome      | Count = 5                   |
| `000000004`        | 429 Rate Limit            | 429 Rate Limit            | Count = 42                  |
| `000000005`        | 500 Server Error          | 500 Server Error          | 400 OperationOutcome        |
| Default            | 202 Accepted (GUID)       | 200 OK (transaction-response) | Count = 0               |

For DELETE `/fhir/Subscription/:id`:
- `00000000-0000-0000-0000-000000000004` → 404 Not Found
- `00000000-0000-0000-0000-000000000005` → 500 Server Error
- Any other ID → 204 No Content

## Configuring mitz-connector

Point the connector at this mock server:

```env
# SOAP endpoints
MITZ_ENDPOINT=https://localhost:8443/xacml
MITZ_OPEN_ENDPOINT=https://localhost:8443/xcpd

# FHIR endpoints
MITZ_FHIR_ENDPOINT=https://localhost:8443/fhir
MITZ_OTV_FHIR_ENDPOINT=https://localhost:8443/fhir
MITZ_FHIR_MTLS_ENABLED=true

# mTLS certificates
MITZ_CERT_PATH=./certs/client.crt
MITZ_KEY_PATH=./certs/client.key
MITZ_CA_PATH=./certs/ca.crt

# SAML auth for OTV-TR-0120/0130/0150
MITZ_FHIR_SAML_AUTH_ENABLED=true
```

## Project Structure

```
mitz-replicator/
├── main.go              # Gin server, TLS config, template loading
├── auth/
│   └── saml.go          # SAML assertion validator + Gin middleware
├── handlers/
│   ├── health.go        # HEAD /xacml
│   ├── xacml.go         # POST /xacml with BSN routing
│   ├── xcpd.go          # POST /xcpd with BSN routing
│   └── fhir.go          # FHIR endpoints with BSN routing
├── parser/
│   ├── request.go       # XACML + XCPD request parsing
│   └── fhir.go          # FHIR Subscription + Bundle parsing
├── templates/
│   ├── xacml_response.xml
│   ├── xacml_fault.xml
│   ├── xcpd_found.xml
│   ├── xcpd_empty.xml
│   ├── xcpd_fault.xml
│   ├── fhir_subscription.xml
│   ├── fhir_bundle_response.xml
│   ├── fhir_processing_status.xml
│   └── fhir_operation_outcome.xml
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

### FHIR — Create Subscription (OTV-TR-0120)

```bash
curl -sk -X POST https://localhost:8443/fhir/Subscription \
  -H "Content-Type: application/fhir+xml; charset=utf-8" \
  -H "X-Request-Id: test-003" \
  -H "Authorization: SAML dGVzdA==" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<Subscription xmlns="http://hl7.org/fhir">
  <status value="requested"/>
  <criteria value="Consent?_query=otv&amp;patientid=000000001&amp;providerid=12345678&amp;providertype=Z3"/>
  <channel>
    <type value="rest-hook"/>
    <endpoint value="https://my-system.example.com/fhir/notificatie"/>
    <payload value="application/fhir+xml"/>
  </channel>
</Subscription>'
```

### FHIR — Cancel Subscription (OTV-TR-0130)

```bash
curl -sk -X DELETE https://localhost:8443/fhir/Subscription/some-guid-here \
  -H "X-Request-Id: test-004" \
  -H "Authorization: SAML dGVzdA=="
```

### FHIR — Bundle Transaction (migration or toestemmingsknop)

```bash
curl -sk -X POST https://localhost:8443/fhir/ \
  -H "Content-Type: application/fhir+xml; charset=utf-8" \
  -H "X-Request-Id: test-005" \
  -H "Authorization: SAML dGVzdA==" \
  -d '<?xml version="1.0" encoding="UTF-8"?>
<Bundle xmlns="http://hl7.org/fhir">
  <type value="transaction"/>
  <entry>
    <resource>
      <Patient>
        <identifier>
          <system value="http://fhir.nl/fhir/NamingSystem/bsn"/>
          <value value="000000001"/>
        </identifier>
        <birthDate value="1990-01-01"/>
      </Patient>
    </resource>
  </entry>
  <entry>
    <resource>
      <Consent>
        <status value="active"/>
      </Consent>
    </resource>
  </entry>
</Bundle>'
```

### FHIR — Query Processing Status

```bash
curl -sk https://localhost:8443/fhir/Subscription/\$processingStatus?providerid=12345678 \
  -H "X-Request-Id: test-006"
```
