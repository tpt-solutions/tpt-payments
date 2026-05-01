"""TPT Payments Python SDK Client"""

import asyncio
import aiohttp
import json
from typing import Optional, Dict, Any, Union

class TPTPay:
    """TPT Payments API client"""
    
    api_base: str = "https://api.tpt-payments.com/v1"
    
    def __init__(self, api_key: str, api_base: Optional[str] = None):
        self.api_key = api_key
        if api_base:
            self.api_base = api_base
            
        # Initialize resources
        self.payment_intents = PaymentIntents(self)
        self.refunds = Refunds(self)
        self.subscriptions = Subscriptions(self)
        self.invoices = Invoices(self)
        
        # Session for async requests
        self._session: Optional[aiohttp.ClientSession] = None
    
    async def _request(self, method: str, path: str, params: Optional[Dict[str, Any]] = None, data: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Make API request"""
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
            "Idempotency-Key": "",
        }
        
        if not self._session:
            self._session = aiohttp.ClientSession()
            
        url = f"{self.api_base}{path}"
        
        async with self._session.request(
            method=method,
            url=url,
            headers=headers,
            params=params,
            json=data
        ) as response:
            response.raise_for_status()
            return await response.json()
    
    async def close(self) -> None:
        """Close HTTP session"""
        if self._session:
            await self._session.close()
            self._session = None
    
    async def __aenter__(self) -> "TPTPay":
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        await self.close()


class APIResource:
    """Base class for API resources"""
    
    def __init__(self, client: TPTPay):
        self.client = client


class PaymentIntents(APIResource):
    """Payment Intents API resource"""
    
    async def create(self, **kwargs) -> Dict[str, Any]:
        """Create a Payment Intent"""
        return await self.client._request("POST", "/payment_intents", data=kwargs)
    
    async def retrieve(self, payment_intent_id: str) -> Dict[str, Any]:
        """Retrieve a Payment Intent"""
        return await self.client._request("GET", f"/payment_intents/{payment_intent_id}")
    
    async def confirm(self, payment_intent_id: str, **kwargs) -> Dict[str, Any]:
        """Confirm a Payment Intent"""
        return await self.client._request("POST", f"/payment_intents/{payment_intent_id}/confirm", data=kwargs)
    
    async def capture(self, payment_intent_id: str, **kwargs) -> Dict[str, Any]:
        """Capture a Payment Intent"""
        return await self.client._request("POST", f"/payment_intents/{payment_intent_id}/capture", data=kwargs)
    
    async def cancel(self, payment_intent_id: str, **kwargs) -> Dict[str, Any]:
        """Cancel a Payment Intent"""
        return await self.client._request("POST", f"/payment_intents/{payment_intent_id}/cancel", data=kwargs)
    
    async def list(self, **kwargs) -> Dict[str, Any]:
        """List Payment Intents"""
        return await self.client._request("GET", "/payment_intents", params=kwargs)


class Refunds(APIResource):
    """Refunds API resource"""
    
    async def create(self, **kwargs) -> Dict[str, Any]:
        """Create a Refund"""
        return await self.client._request("POST", "/refunds", data=kwargs)
    
    async def retrieve(self, refund_id: str) -> Dict[str, Any]:
        """Retrieve a Refund"""
        return await self.client._request("GET", f"/refunds/{refund_id}")
    
    async def list(self, **kwargs) -> Dict[str, Any]:
        """List Refunds"""
        return await self.client._request("GET", "/refunds", params=kwargs)


class Subscriptions(APIResource):
    """Subscriptions API resource"""
    
    async def create(self, **kwargs) -> Dict[str, Any]:
        """Create a Subscription"""
        return await self.client._request("POST", "/subscriptions", data=kwargs)
    
    async def cancel(self, subscription_id: str, at_period_end: bool = False) -> Dict[str, Any]:
        """Cancel a Subscription"""
        return await self.client._request("POST", f"/subscriptions/{subscription_id}/cancel", data={"at_period_end": at_period_end})


class Invoices(APIResource):
    """Invoices API resource"""
    
    async def retrieve(self, invoice_id: str) -> Dict[str, Any]:
        """Retrieve an Invoice"""
        return await self.client._request("GET", f"/invoices/{invoice_id}")
    
    async def pay(self, invoice_id: str) -> Dict[str, Any]:
        """Pay an Invoice"""
        return await self.client._request("POST", f"/invoices/{invoice_id}/pay")