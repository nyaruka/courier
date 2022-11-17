#!/usr/bin/env python3
import json
import requests


response = requests.get("https://www.twilio.com/docs/documents/76/twilio-error-codes.json")
codes = {str(e["code"]): e["message"] for e in response.json()}
with open("../handlers/twiml/errors.json", "w") as f:
    f.write(json.dumps(codes))
