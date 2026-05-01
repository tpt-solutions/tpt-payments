"""Webhook signature verification utilities"""

import hmac
import hashlib
import json
from typing import Dict, Any

def verify_signature(payload: bytes, signature_header: str, secret: str, tolerance: int = 300) -> bool:
    """Verify webhook signature
    
    Args:
        payload: Raw request body bytes
        signature_header: Stripe-Signature header value
        secret: Webhook secret
        tolerance: Time tolerance in seconds (default 5 minutes)
        
    Returns:
        True if signature is valid
        
    Raises:
        ValueError: If signature format is invalid
    """
    parts = {}
    for part in signature_header.split(','):
        key, value = part.split('=', 1)
        parts[key] = value
        
    if 't' not in parts or 'v1' not in parts:
        raise ValueError("Invalid signature header format")
        
    timestamp = int(parts['t'])
    import time
    current_time = int(time.time())
    
    if abs(current_time - timestamp) > tolerance:
        return False
        
    signed_payload = f"{parts['t']}.{payload.decode('utf-8')}"
    expected_signature = hmac.new(
        secret.encode('utf-8'),
        signed_payload.encode('utf-8'),
        hashlib.sha256
    ).hexdigest()
    
    return hmac.compare_digest(parts['v1'], expected_signature)


def construct_event(payload: bytes, signature_header: str, secret: str) -> Dict[str, Any]:
    """Construct and verify webhook event
    
    Args:
        payload: Raw request body bytes
        signature_header: Stripe-Signature header value
        secret: Webhook secret
        
    Returns:
        Parsed event object
        
    Raises:
        ValueError: If signature is invalid or event cannot be parsed
    """
    if not verify_signature(payload, signature_header, secret):
        raise ValueError("Invalid webhook signature")
        
    return json.loads(payload)