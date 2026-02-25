# Mitz Replicator — Requirements & Mock Responses

A lightweight Go (Gin) server that mimics the VZVZ Mitz consent register for local testing of `mitz-connector`.

---

## 1. Endpoints

| Method | Path     | Purpose                                      |
|--------|----------|----------------------------------------------|
| HEAD   | `/xacml` | Health-check probe (mTLS connectivity test)  |
| POST   | `/xacml` | Gesloten autorisatievraag (XACML 3.0 / 2.0) |
| POST   | `/xcpd`  | Open autorisatievraag (XCPD + XUA SAML)      |

All POST endpoints accept and return `Content-Type: application/soap+xml; charset=utf-8`.

---

## 2. TLS / mTLS

The connector always connects over HTTPS with mutual TLS:

- The replicator must serve HTTPS with a self-signed server certificate.
- It should optionally verify the client certificate (mTLS) — or skip verification for local dev.
- Minimum TLS version: 1.2.

For local testing, generate a CA + server cert + client cert:

```bash
# CA
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -out ca.crt -days 365 -subj "/CN=Mitz Test CA"

# Server
openssl req -newkey rsa:2048 -nodes -keyout server.key -out server.csr -subj "/CN=localhost"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365

# Client (for mitz-connector)
openssl req -newkey rsa:2048 -nodes -keyout client.key -out client.csr -subj "/CN=mitz-connector"
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt -days 365
```

Configure `mitz-connector` with:

```env
MITZ_ENDPOINT=https://localhost:8443/xacml
MITZ_OPEN_ENDPOINT=https://localhost:8443/xcpd
MITZ_CERT_PATH=./certs/client.crt
MITZ_KEY_PATH=./certs/client.key
MITZ_CA_PATH=./certs/ca.crt
```

---

## 3. HEAD /xacml — Health Check

The connector sends a TLS HEAD request with the mTLS client certificate. No body, no special headers.

**Response:** Return `200 OK` with an empty body. That's all the connector needs.

---

## 4. POST /xacml — Gesloten Autorisatievraag

### 4.1 Incoming Request

The connector sends a SOAP 1.2 envelope containing an XACML authorization decision query.

**Headers received:**
- `Content-Type: application/soap+xml; charset=utf-8`
- `X-Request-Id: <uuid>` (correlation)
- `X-Trace-Id: <uuid>` (optional)

**XACML 3.0 request body** (most common):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Header/>
  <soap:Body>
    <xacml-samlp:XACMLAuthzDecisionQuery
        xmlns:xacml-samlp="urn:oasis:names:tc:xacml:3.0:profile:saml2.0:v2:schema:protocol"
        xmlns:xacml-context="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
      <xacml-context:Request ReturnPolicyIdList="false" CombinedDecision="false">

        <!-- Resource: patient + dossierhouder -->
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource">
          <xacml-context:Attribute
              AttributeId="urn:oasis:names:tc:xacml:2.0:resource:resource-id"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">999999999</xacml-context:AttributeValue>
          </xacml-context:Attribute>
          <xacml-context:Attribute
              AttributeId="urn:ihe:iti:appc:2016:author-institution:id"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">2.16.528.1.1007.3.3^00001234</xacml-context:AttributeValue>
          </xacml-context:Attribute>
          <xacml-context:Attribute
              AttributeId="urn:ihe:iti:appc:2016:document-entry:healthcare-facility-type-code"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.15.1060^01</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>

        <!-- Action: one element per gegevenscategorie (multi-decision) -->
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute
              AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.3.111.5.10.1^huisartsgegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute
              AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.3.111.5.10.1^medicatiegegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>

        <!-- Subject: raadpleger identity -->
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:1.0:subject-category:access-subject">
          <xacml-context:Attribute
              AttributeId="urn:ihe:iti:xua:2017:subject:provider-identifier"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">UZI-12345</xacml-context:AttributeValue>
          </xacml-context:Attribute>
          <xacml-context:Attribute
              AttributeId="urn:oasis:names:tc:xacml:2.0:subject:role"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.15.111^01.015</xacml-context:AttributeValue>
          </xacml-context:Attribute>
          <!-- ... more subject attributes ... -->
        </xacml-context:Attributes>

        <!-- Environment: purpose of use -->
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:environment">
          <xacml-context:Attribute
              AttributeId="urn:oasis:names:tc:xspa:1.0:subject:purposeofuse"
              IncludeInResult="true">
            <xacml-context:AttributeValue
                DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.1.11.20448^TREAT</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>

      </xacml-context:Request>
    </xacml-samlp:XACMLAuthzDecisionQuery>
  </soap:Body>
</soap:Envelope>
```

### 4.2 Parsing the Request (what to extract)

To build dynamic responses, parse these from the incoming XACML request:

| Field | XPath | Example |
|-------|-------|---------|
| Patient BSN | `//Attributes[@Category="...resource"]/Attribute[@AttributeId="...resource-id"]/AttributeValue` | `999999999` |
| Gegevenscategorieen | `//Attributes[@Category="...action"]/Attribute[@AttributeId="...event-code"]/AttributeValue` | `2.16.840.1.113883.2.4.3.111.5.10.1^huisartsgegevens` |

The number of Action `<Attributes>` elements tells you how many `<Result>` elements to return (one per gegevenscategorie).

### 4.3 Success Responses

#### Permit All (simplest)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <xacml-context:Response xmlns:xacml-context="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
      <xacml-context:Result>
        <xacml-context:Decision>Permit</xacml-context:Decision>
        <xacml-context:Status>
          <xacml-context:StatusCode Value="urn:oasis:names:tc:xacml:1.0:status:ok"/>
        </xacml-context:Status>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code">
            <xacml-context:AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.3.111.5.10.1^huisartsgegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
      </xacml-context:Result>
      <xacml-context:Result>
        <xacml-context:Decision>Permit</xacml-context:Decision>
        <xacml-context:Status>
          <xacml-context:StatusCode Value="urn:oasis:names:tc:xacml:1.0:status:ok"/>
        </xacml-context:Status>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code">
            <xacml-context:AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.3.111.5.10.1^medicatiegegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
      </xacml-context:Result>
    </xacml-context:Response>
  </soap:Body>
</soap:Envelope>
```

#### Mixed Decisions (Permit + Deny)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <xacml-context:Response xmlns:xacml-context="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
      <xacml-context:Result>
        <xacml-context:Decision>Permit</xacml-context:Decision>
        <xacml-context:Status>
          <xacml-context:StatusCode Value="urn:oasis:names:tc:xacml:1.0:status:ok"/>
        </xacml-context:Status>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code">
            <xacml-context:AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.3.111.5.10.1^huisartsgegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
      </xacml-context:Result>
      <xacml-context:Result>
        <xacml-context:Decision>Deny</xacml-context:Decision>
        <xacml-context:Status>
          <xacml-context:StatusCode Value="urn:oasis:names:tc:xacml:1.0:status:ok"/>
        </xacml-context:Status>
        <xacml-context:Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
          <xacml-context:Attribute AttributeId="urn:ihe:iti:appc:2016:document-entry:event-code">
            <xacml-context:AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">2.16.840.1.113883.2.4.3.111.5.10.1^medicatiegegevens</xacml-context:AttributeValue>
          </xacml-context:Attribute>
        </xacml-context:Attributes>
      </xacml-context:Result>
    </xacml-context:Response>
  </soap:Body>
</soap:Envelope>
```

#### Deny All

Same structure, all `<Decision>Deny</Decision>`.

#### Indeterminate / NotApplicable

Valid decision values: `Permit`, `Deny`, `Indeterminate`, `NotApplicable`. Any unknown value is treated as `Indeterminate` by the connector.

#### Minimal Response (no Attributes echo)

The connector falls back to index-based category matching if no Attributes are echoed. This is the absolute minimum:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <xacml-context:Response xmlns:xacml-context="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
      <xacml-context:Result>
        <xacml-context:Decision>Permit</xacml-context:Decision>
      </xacml-context:Result>
    </xacml-context:Response>
  </soap:Body>
</soap:Envelope>
```

### 4.4 SOAP Fault Response

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <soap:Fault>
      <soap:Code>
        <soap:Value>soap:Sender</soap:Value>
        <soap:Subcode>
          <soap:Value>mitz:InvalidRequest</soap:Value>
        </soap:Subcode>
      </soap:Code>
      <soap:Reason>
        <soap:Text>Patient BSN not found in register</soap:Text>
      </soap:Reason>
      <soap:Detail>Additional error details here</soap:Detail>
    </soap:Fault>
  </soap:Body>
</soap:Envelope>
```

The connector translates this to HTTP 502 with:
```json
{
  "statusCode": 502,
  "error": "SOAP Fault",
  "message": "Patient BSN not found in register",
  "details": {
    "code": "soap:Sender",
    "subcode": "mitz:InvalidRequest",
    "detail": "Additional error details here"
  }
}
```

---

## 5. POST /xcpd — Open Autorisatievraag

### 5.1 Incoming Request

The connector sends a SOAP envelope with a WS-Security header (signed SAML 2.0 assertion) and an HL7v3 XCPD body.

**Headers received:** Same as `/xacml`.

**Request body:**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Header>
    <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
                   xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
      <saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"
                      ID="_550e8400-e29b-41d4-a716-446655440000"
                      Version="2.0"
                      IssueInstant="2026-02-25T12:00:00.000Z">
        <!-- Signed SAML assertion — the mock can safely ignore this -->
      </saml:Assertion>
    </wsse:Security>
  </soap:Header>
  <soap:Body>
    <PRPA_IN201305UV02 xmlns="urn:hl7-org:v3" ITSVersion="XML_1.0">
      <id root="550e8400-e29b-41d4-a716-446655440000"/>
      <creationTime value="20260225120000"/>
      <interactionId root="2.16.840.1.113883.1.6" extension="PRPA_IN201305UV02"/>
      <processingCode code="P"/>
      <processingModeCode code="T"/>
      <acceptAckCode code="AL"/>
      <receiver typeCode="RCV">
        <device classCode="DEV" determinerCode="INSTANCE">
          <id root="Mitz"/>
        </device>
      </receiver>
      <sender typeCode="SND">
        <device classCode="DEV" determinerCode="INSTANCE">
          <id root="00005678"/>
        </device>
      </sender>
      <controlActProcess classCode="CACT" moodCode="EVN">
        <code code="PRPA_IN201305UV02" codeSystem="2.16.840.1.113883.1.6"/>
        <queryByParameter>
          <queryId root="request-uuid-here"/>
          <statusCode code="new"/>
          <parameterList>
            <livingSubjectId>
              <value root="2.16.840.1.113883.2.4.6.3" extension="999999999"/>
              <semanticsText>LivingSubject.id</semanticsText>
            </livingSubjectId>
          </parameterList>
        </queryByParameter>
      </controlActProcess>
    </PRPA_IN201305UV02>
  </soap:Body>
</soap:Envelope>
```

### 5.2 Parsing the Request

| Field | XPath | Example |
|-------|-------|---------|
| Patient BSN | `//livingSubjectId/value/@extension` | `999999999` |
| Sender org | `//sender/device/id/@root` | `00005678` |

### 5.3 Success Response — Patient Found

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <PRPA_IN201306UV02 xmlns="urn:hl7-org:v3" ITSVersion="XML_1.0">
      <id root="response-uuid"/>
      <creationTime value="20260225120001"/>
      <interactionId root="2.16.840.1.113883.1.6" extension="PRPA_IN201306UV02"/>
      <processingCode code="P"/>
      <processingModeCode code="T"/>
      <acceptAckCode code="NE"/>
      <acknowledgement>
        <typeCode code="AA"/>
      </acknowledgement>
      <controlActProcess classCode="CACT" moodCode="EVN">

        <!-- Location 1: Hospital with huisartsgegevens -->
        <subject typeCode="SUBJ">
          <registrationEvent classCode="REG" moodCode="EVN">
            <subject1 typeCode="SBJ">
              <patient classCode="PAT">
                <id root="2.16.840.1.113883.2.4.6.3" extension="123456789"/>
                <id root="1.2.3.4.5.6.7"/>
              </patient>
            </subject1>
            <custodian typeCode="CST">
              <assignedEntity classCode="ASSIGNED">
                <id root="urn:oid:2.16.840.1.113883.2.4.6.6"/>
              </assignedEntity>
            </custodian>
          </registrationEvent>
          <queryMatchObservation>
            <value code="huisartsgegevens"/>
          </queryMatchObservation>
          <queryMatchObservation>
            <value code="medicatiegegevens"/>
          </queryMatchObservation>
          <queryByParameter>
            <livingSubjectId>
              <value root="2.16.840.1.113883.2.4.6.3" extension="999999999"/>
            </livingSubjectId>
          </queryByParameter>
        </subject>

        <!-- Location 2: Pharmacy with medicatiegegevens -->
        <subject typeCode="SUBJ">
          <registrationEvent classCode="REG" moodCode="EVN">
            <subject1 typeCode="SBJ">
              <patient classCode="PAT">
                <id root="2.16.840.1.113883.2.4.6.3" extension="987654321"/>
              </patient>
            </subject1>
            <custodian typeCode="CST">
              <assignedEntity classCode="ASSIGNED">
                <id root="urn:oid:2.16.840.1.113883.2.4.3.11"/>
              </assignedEntity>
            </custodian>
          </registrationEvent>
          <queryMatchObservation>
            <value code="medicatiegegevens"/>
          </queryMatchObservation>
          <queryByParameter>
            <livingSubjectId>
              <value root="2.16.840.1.113883.2.4.6.3" extension="999999999"/>
            </livingSubjectId>
          </queryByParameter>
        </subject>

      </controlActProcess>
    </PRPA_IN201306UV02>
  </soap:Body>
</soap:Envelope>
```

The connector maps this to:
```json
{
  "requestId": "...",
  "timestamp": "...",
  "locaties": [
    {
      "homeCommunityId": "urn:oid:2.16.840.1.113883.2.4.6.6",
      "correspondingPatientId": "2.16.840.1.113883.2.4.6.3^^^&2.16.840.1.113883.2.4.6.3&ISO",
      "requestedPatientId": "999999999",
      "sourceId": "1.2.3.4.5.6.7",
      "eventCodes": ["huisartsgegevens", "medicatiegegevens"]
    },
    {
      "homeCommunityId": "urn:oid:2.16.840.1.113883.2.4.3.11",
      "correspondingPatientId": "2.16.840.1.113883.2.4.6.3^^^&2.16.840.1.113883.2.4.6.3&ISO",
      "requestedPatientId": "999999999",
      "sourceId": null,
      "eventCodes": ["medicatiegegevens"]
    }
  ]
}
```

### 5.4 Empty Response — Patient Not Found

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    <PRPA_IN201306UV02 xmlns="urn:hl7-org:v3">
      <controlActProcess classCode="CACT" moodCode="EVN">
      </controlActProcess>
    </PRPA_IN201306UV02>
  </soap:Body>
</soap:Envelope>
```

Returns `{ "locaties": [] }`.

### 5.5 SOAP Fault — same format as `/xacml`

---

## 6. Suggested Mock Behavior

### 6.1 BSN-Based Routing

Use the patient BSN to determine the response:

| BSN Pattern | /xacml Response | /xcpd Response |
|-------------|-----------------|----------------|
| `000000001` | All Permit | 2 locations with multiple event codes |
| `000000002` | All Deny | 1 location with 1 event code |
| `000000003` | Mixed (Permit + Deny) | Empty (patient not found) |
| `000000004` | All Indeterminate | SOAP Fault |
| `000000005` | SOAP Fault | SOAP Fault |
| `999*` (default) | All Permit | 1 location with all requested event codes |

### 6.2 Dynamic XACML Response Generation

For `/xacml`, parse the incoming request to extract the gegevenscategorieen from the Action `<Attributes>` elements, then return one `<Result>` per categorie with the event-code echoed in the response Attributes. This ensures the connector's mapper correctly matches results to categories.

### 6.3 Request Logging

Log every incoming request with:
- Timestamp
- Method + path
- X-Request-Id header
- Extracted BSN
- Extracted gegevenscategorieen (for /xacml) or sender org (for /xcpd)

---

## 7. OID Quick Reference

| Name | OID |
|------|-----|
| BSN | `2.16.840.1.113883.2.4.6.3` |
| URA Register | `2.16.528.1.1007.3.3` |
| UZI Rolcode | `2.16.840.1.113883.2.4.15.111` |
| Zorgaanbieder Categorie | `2.16.840.1.113883.2.4.15.1060` |
| Mitz Gegevenscategorie | `2.16.840.1.113883.2.4.3.111.5.10.1` |
| Purpose of Use | `2.16.840.1.113883.1.11.20448` |
| HL7 Interaction | `2.16.840.1.113883.1.6` |

---

## 8. Namespace Quick Reference

| Prefix | URI |
|--------|-----|
| `soap` | `http://www.w3.org/2003/05/soap-envelope` |
| `xacml-context` (3.0) | `urn:oasis:names:tc:xacml:3.0:core:schema:wd-17` |
| `xacml-context` (2.0) | `urn:oasis:names:tc:xacml:2.0:context:schema:os` |
| `xacml-samlp` (3.0) | `urn:oasis:names:tc:xacml:3.0:profile:saml2.0:v2:schema:protocol` |
| `xacml-samlp` (2.0) | `urn:oasis:names:tc:xacml:2.0:profile:saml2.0:v2:schema:protocol` |
| `saml` | `urn:oasis:names:tc:SAML:2.0:assertion` |
| `wsse` | `http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd` |
| `hl7v3` | `urn:hl7-org:v3` |

---

## 9. Go Project Skeleton

```
mitz-replicator/
├── main.go                  # Gin server setup, TLS config
├── handlers/
│   ├── xacml.go             # POST /xacml handler
│   ├── xcpd.go              # POST /xcpd handler
│   └── health.go            # HEAD /xacml handler
├── templates/
│   ├── xacml_permit.xml     # XACML Permit response template
│   ├── xacml_deny.xml       # XACML Deny response template
│   ├── xacml_fault.xml      # SOAP Fault template
│   ├── xcpd_found.xml       # XCPD patient found template
│   ├── xcpd_empty.xml       # XCPD patient not found template
│   └── xcpd_fault.xml       # XCPD SOAP Fault template
├── parser/
│   └── request.go           # Extract BSN + categorieen from SOAP XML
├── certs/                   # Self-signed test certificates
│   ├── ca.crt
│   ├── server.crt
│   ├── server.key
│   ├── client.crt
│   └── client.key
├── go.mod
└── go.sum
```
