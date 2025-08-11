#!/usr/bin/env python3
"""
🤖 Standalone AI Agent Service
Advanced AI command processing service with multiple LLM providers
Provides enhanced natural language understanding for ad queue management
"""

import asyncio
import json
import logging
import os
import time
from datetime import datetime
from typing import Dict, List, Optional, Union
from contextlib import asynccontextmanager

import httpx
import redis.asyncio as redis
import structlog
import uvicorn
from fastapi import FastAPI, HTTPException, Request, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, Response
from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST
from pydantic import BaseModel, Field
from tenacity import retry, stop_after_attempt, wait_exponential

# Configure logging
structlog.configure(
    processors=[
        structlog.stdlib.filter_by_level,
        structlog.stdlib.add_logger_name,
        structlog.stdlib.add_log_level,
        structlog.stdlib.PositionalArgumentsFormatter(),
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
        structlog.processors.UnicodeDecoder(),
        structlog.processors.JSONRenderer()
    ],
    context_class=dict,
    logger_factory=structlog.stdlib.LoggerFactory(),
    cache_logger_on_first_use=True,
)

logger = structlog.get_logger(__name__)

# Configuration
class Config:
    # Service configuration
    SERVICE_NAME = os.getenv("SERVICE_NAME", "ai-agent")
    SERVICE_VERSION = os.getenv("SERVICE_VERSION", "1.0.0")
    HOST = os.getenv("HOST", "0.0.0.0")
    PORT = int(os.getenv("PORT", "8080"))
    
    # AI Provider configuration
    GOOGLE_API_KEY = os.getenv("GOOGLE_API_KEY", "")
    OPENAI_API_KEY = os.getenv("OPENAI_API_KEY", "")
    ANTHROPIC_API_KEY = os.getenv("ANTHROPIC_API_KEY", "")
    DEFAULT_PROVIDER = os.getenv("AI_PROVIDER", "google")
    
    # Redis configuration
    REDIS_HOST = os.getenv("REDIS_HOST", "localhost")
    REDIS_PORT = int(os.getenv("REDIS_PORT", "6380"))
    REDIS_PASSWORD = os.getenv("REDIS_PASSWORD", "")
    REDIS_DB = int(os.getenv("REDIS_DB", "0"))
    
    # API Gateway configuration
    AD_API_URL = os.getenv("AD_API_URL", "http://localhost:8443/api")
    API_TIMEOUT = int(os.getenv("API_TIMEOUT", "30"))
    
    # Performance configuration
    MAX_CONCURRENT_REQUESTS = int(os.getenv("MAX_CONCURRENT_REQUESTS", "100"))
    CACHE_TTL = int(os.getenv("CACHE_TTL", "300"))
    RATE_LIMIT_PER_MINUTE = int(os.getenv("RATE_LIMIT_PER_MINUTE", "100"))

config = Config()

# Prometheus metrics - disable to avoid registration conflicts
METRICS = {
    'requests_total': None,  # Disabled to avoid conflicts
    'request_duration': None,
    'ai_processing_duration': None,
    'cache_hits': None,
    'cache_misses': None,
    'active_requests': None,
    'ai_provider_errors': None,
}

# Helper function to safely use metrics
def safe_metric_call(metric_name, method_name='inc', *args, **kwargs):
    """Safely call metric methods, ignoring if metrics are disabled"""
    metric = METRICS.get(metric_name)
    if metric is not None:
        try:
            method = getattr(metric, method_name)
            return method(*args, **kwargs)
        except Exception:
            pass  # Ignore metric errors
    return None

# Pydantic models
class CommandRequest(BaseModel):
    command: str = Field(..., min_length=1, max_length=1000, description="Natural language command")
    context: Optional[Dict] = Field(default=None, description="Additional context")
    user_id: Optional[str] = Field(default=None, description="User identifier")
    priority: Optional[int] = Field(default=3, ge=1, le=5, description="Processing priority")

class CommandResponse(BaseModel):
    command_id: str = Field(..., description="Unique command identifier")
    status: str = Field(..., description="Processing status")
    intent: str = Field(..., description="Parsed command intent")
    command_type: str = Field(..., description="Command type category")
    parameters: Dict = Field(..., description="Extracted parameters")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Parsing confidence")
    processing_time_ms: int = Field(..., description="Processing time in milliseconds")
    provider: str = Field(..., description="AI provider used")
    result: Optional[Dict] = Field(default=None, description="Execution result")

class HealthResponse(BaseModel):
    status: str
    version: str
    uptime_seconds: int
    ai_providers: Dict[str, str]
    cache_status: str

# AI Provider interfaces
class AIProvider:
    """Base class for AI providers"""
    
    async def parse_command(self, command: str, context: Optional[Dict] = None) -> Dict:
        raise NotImplementedError
    
    async def health_check(self) -> bool:
        raise NotImplementedError

class GoogleAIProvider(AIProvider):
    """Google AI (Gemini) provider"""
    
    def __init__(self, api_key: str):
        self.api_key = api_key
        self.endpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash-latest:generateContent"
        
    @retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=4, max=10))
    async def parse_command(self, command: str, context: Optional[Dict] = None) -> Dict:
        prompt = self._build_prompt(command, context)
        
        payload = {
            "contents": [{"parts": [{"text": prompt}]}],
            "generationConfig": {
                "temperature": 0.1,
                "topK": 40,
                "topP": 0.95,
                "maxOutputTokens": 1024,
            }
        }
        
        async with httpx.AsyncClient(timeout=config.API_TIMEOUT) as client:
            response = await client.post(
                f"{self.endpoint}?key={self.api_key}",
                json=payload,
                headers={"Content-Type": "application/json"}
            )
            
        if response.status_code != 200:
            raise HTTPException(status_code=500, detail=f"Google AI API error: {response.status_code}")
            
        result = response.json()
        
        if 'candidates' not in result or not result['candidates']:
            raise HTTPException(status_code=500, detail="No response from Google AI")
            
        content = result['candidates'][0]['content']['parts'][0]['text']
        return json.loads(content)
    
    def _build_prompt(self, command: str, context: Optional[Dict] = None) -> str:
        context_str = ""
        if context:
            context_str = f"\nContext: {json.dumps(context, indent=2)}\n"
        
        return f"""You are an AI assistant that parses natural language commands for an ad processing queue system.

Parse the following command and return a JSON response with the command details:

Command: "{command}"{context_str}

Supported command types and their formats:

1. Queue Modification Commands:
   - "Change priority to {{X}} for all ads in the {{gameFamily}} family"
   - "Set priority to {{X}} for ads older than {{Y}} minutes"
   - "Boost priority for ads waiting longer than {{X}} minutes"
   - "Remove ads from {{gameFamily}} family from queue"
   
2. System Configuration Commands:
   - "Enable starvation mode"
   - "Disable starvation mode"
   - "Set maximum wait time to {{X}} seconds"
   - "Set worker count to {{X}}"
   - "Pause queue processing"
   - "Resume queue processing"
   
3. Status and Analytics Commands:
   - "Show the next {{X}} ads to be processed"
   - "List all ads waiting longer than {{X}} minutes"
   - "What's the current queue distribution by priority?"
   - "Show queue performance summary"
   - "Get processing statistics"
   - "Show ads by game family {{gameFamily}}"
   
4. Advanced Commands:
   - "Create performance report for last {{X}} hours"
   - "Export queue data to CSV"
   - "Predict processing time for priority {{X}}"
   - "Optimize queue for maximum throughput"

Return ONLY a JSON object in this format:
{{
  "intent": "specific_action_name",
  "command_type": "queue_modification|system_configuration|status_query|analytics|advanced",
  "parameters": {{
    "parameter_name": "parameter_value"
  }},
  "confidence": 0.95,
  "valid": true,
  "error": null,
  "reasoning": "Brief explanation of the parsing"
}}

If the command cannot be parsed, return:
{{
  "intent": "unknown",
  "command_type": "unknown", 
  "parameters": {{}},
  "confidence": 0.0,
  "valid": false,
  "error": "Detailed error description",
  "reasoning": "Why the command couldn't be parsed"
}}

Priority values must be between 1-5. Validate all numeric parameters. Provide confidence score (0.0-1.0)."""

    async def health_check(self) -> bool:
        try:
            async with httpx.AsyncClient(timeout=5) as client:
                response = await client.get(f"{self.endpoint.split('/v1beta')[0]}")
                return response.status_code < 500
        except:
            return False

# Global variables
app_start_time = time.time()
redis_client: Optional[redis.Redis] = None
ai_providers: Dict[str, AIProvider] = {}
active_requests = 0

# Lifespan management
@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup
    global redis_client, ai_providers
    
    logger.info("Starting AI Agent Service", version=config.SERVICE_VERSION)
    
    # Initialize Redis
    try:
        redis_client = redis.Redis(
            host=config.REDIS_HOST,
            port=config.REDIS_PORT,
            password=config.REDIS_PASSWORD,
            db=config.REDIS_DB,
            decode_responses=True
        )
        await redis_client.ping()
        logger.info("Redis connection established")
    except Exception as e:
        logger.error("Failed to connect to Redis", error=str(e))
        redis_client = None
    
    # Initialize AI providers
    if config.GOOGLE_API_KEY:
        ai_providers["google"] = GoogleAIProvider(config.GOOGLE_API_KEY)
        logger.info("Google AI provider initialized")
    
    # Add other providers here (OpenAI, Anthropic, etc.)
    
    if not ai_providers:
        logger.warning("No AI providers configured")
    
    logger.info("AI Agent Service started successfully")
    
    yield
    
    # Shutdown
    if redis_client:
        await redis_client.close()
    logger.info("AI Agent Service shutdown complete")

# Create FastAPI app
app = FastAPI(
    title="AI Agent Service",
    description="Advanced AI command processing service for ad queue management",
    version=config.SERVICE_VERSION,
    lifespan=lifespan
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Middleware for metrics and logging
@app.middleware("http")
async def metrics_middleware(request: Request, call_next):
    global active_requests
    
    start_time = time.time()
    active_requests += 1
    safe_metric_call('active_requests', 'set', active_requests)
    
    try:
        response = await call_next(request)
        
        # Record metrics
        duration = time.time() - start_time
        safe_metric_call('request_duration', 'observe', duration)
        # Record request metrics
        metric = METRICS.get('requests_total')
        if metric is not None:
            try:
                metric.labels(
                    method=request.method,
                    endpoint=request.url.path,
                    status=response.status_code
                ).inc()
            except Exception:
                pass
        
        return response
    finally:
        active_requests -= 1
        safe_metric_call('active_requests', 'set', active_requests)

# Cache utilities
async def get_from_cache(key: str) -> Optional[Dict]:
    if not redis_client:
        return None
    
    try:
        cached = await redis_client.get(key)
        if cached:
            safe_metric_call('cache_hits', 'inc')
            return json.loads(cached)
        else:
            safe_metric_call('cache_misses', 'inc')
            return None
    except Exception as e:
        logger.error("Cache get error", key=key, error=str(e))
        return None

async def set_in_cache(key: str, value: Dict, ttl: int = None) -> bool:
    if not redis_client:
        return False
    
    try:
        await redis_client.setex(
            key, 
            ttl or config.CACHE_TTL, 
            json.dumps(value)
        )
        return True
    except Exception as e:
        logger.error("Cache set error", key=key, error=str(e))
        return False

# Command processing
async def process_command(request: CommandRequest) -> CommandResponse:
    """Process a command using AI providers with fallback"""
    command_id = f"cmd_{int(time.time() * 1000)}_{hash(request.command) % 1000000}"
    start_time = time.time()
    
    # Check cache first
    cache_key = f"command:{hash(request.command)}"
    if request.context:
        cache_key += f":{hash(str(request.context))}"
    
    cached_result = await get_from_cache(cache_key)
    if cached_result:
        logger.info("Command served from cache", command_id=command_id)
        cached_result['command_id'] = command_id
        cached_result['processing_time_ms'] = int((time.time() - start_time) * 1000)
        return CommandResponse(**cached_result)
    
    # Try AI providers in order of preference
    providers_to_try = [config.DEFAULT_PROVIDER] + [p for p in ai_providers.keys() if p != config.DEFAULT_PROVIDER]
    
    last_error = None
    for provider_name in providers_to_try:
        if provider_name not in ai_providers:
            continue
            
        provider = ai_providers[provider_name]
        ai_start_time = time.time()
        
        try:
            logger.info("Processing command", command_id=command_id, provider=provider_name, command=request.command[:100])
            
            result = await provider.parse_command(request.command, request.context)
            
            ai_duration = time.time() - ai_start_time
            # Record AI processing duration
            metric = METRICS.get('ai_processing_duration')
            if metric is not None:
                try:
                    metric.labels(provider=provider_name).observe(ai_duration)
                except Exception:
                    pass
            
            # Validate result
            if not result.get('valid', False):
                raise ValueError(result.get('error', 'Invalid command parsing result'))
            
            # Create response
            response_data = {
                'command_id': command_id,
                'status': 'completed',
                'intent': result.get('intent', 'unknown'),
                'command_type': result.get('command_type', 'unknown'),
                'parameters': result.get('parameters', {}),
                'confidence': result.get('confidence', 0.0),
                'processing_time_ms': int((time.time() - start_time) * 1000),
                'provider': provider_name
            }
            
            # Cache successful result
            await set_in_cache(cache_key, response_data)
            
            logger.info("Command processed successfully", 
                       command_id=command_id, 
                       provider=provider_name,
                       confidence=result.get('confidence', 0.0))
            
            return CommandResponse(**response_data)
            
        except Exception as e:
            last_error = str(e)
            # Record AI provider error
            metric = METRICS.get('ai_provider_errors')
            if metric is not None:
                try:
                    metric.labels(provider=provider_name, error_type=type(e).__name__).inc()
                except Exception:
                    pass
            logger.error("AI provider failed", 
                        command_id=command_id, 
                        provider=provider_name, 
                        error=str(e))
            continue
    
    # All providers failed
    logger.error("All AI providers failed", command_id=command_id, last_error=last_error)
    
    return CommandResponse(
        command_id=command_id,
        status='error',
        intent='unknown',
        command_type='unknown',
        parameters={},
        confidence=0.0,
        processing_time_ms=int((time.time() - start_time) * 1000),
        provider='none',
        result={'error': f'All AI providers failed. Last error: {last_error}'}
    )

# API Endpoints
@app.get("/", response_model=Dict)
async def root():
    """Root endpoint with service information"""
    return {
        "service": config.SERVICE_NAME,
        "version": config.SERVICE_VERSION,
        "status": "healthy",
        "ai_providers": list(ai_providers.keys()),
        "endpoints": {
            "health": "/health",
            "parse": "/api/v1/parse",
            "batch": "/api/v1/batch",
            "metrics": "/metrics"
        }
    }

@app.get("/health", response_model=HealthResponse)
async def health():
    """Health check endpoint"""
    # Check AI providers
    provider_status = {}
    for name, provider in ai_providers.items():
        try:
            is_healthy = await provider.health_check()
            provider_status[name] = "healthy" if is_healthy else "unhealthy"
        except:
            provider_status[name] = "error"
    
    # Check cache
    cache_status = "healthy"
    if redis_client:
        try:
            await redis_client.ping()
        except:
            cache_status = "unhealthy"
    else:
        cache_status = "disabled"
    
    return HealthResponse(
        status="healthy",
        version=config.SERVICE_VERSION,
        uptime_seconds=int(time.time() - app_start_time),
        ai_providers=provider_status,
        cache_status=cache_status
    )

@app.post("/api/v1/parse", response_model=CommandResponse)
async def parse_command_endpoint(request: CommandRequest):
    """Parse a single command"""
    return await process_command(request)

@app.post("/api/v1/batch")
async def batch_parse(requests: List[CommandRequest]):
    """Parse multiple commands in batch"""
    if len(requests) > 10:
        raise HTTPException(status_code=400, detail="Maximum 10 commands per batch")
    
    tasks = [process_command(req) for req in requests]
    results = await asyncio.gather(*tasks, return_exceptions=True)
    
    # Convert exceptions to error responses
    processed_results = []
    for i, result in enumerate(results):
        if isinstance(result, Exception):
            processed_results.append({
                'command_id': f'batch_error_{i}',
                'status': 'error',
                'intent': 'unknown',
                'command_type': 'unknown', 
                'parameters': {},
                'confidence': 0.0,
                'processing_time_ms': 0,
                'provider': 'none',
                'result': {'error': str(result)}
            })
        else:
            processed_results.append(result.dict())
    
    return {'results': processed_results, 'total': len(requests)}

@app.get("/metrics")
async def metrics():
    """Prometheus metrics endpoint"""
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)

@app.get("/api/v1/stats")
async def stats():
    """Service statistics"""
    return {
        'uptime_seconds': int(time.time() - app_start_time),
        'active_requests': active_requests,
        'ai_providers': len(ai_providers),
        'cache_enabled': redis_client is not None
    }

# Error handlers
@app.exception_handler(HTTPException)
async def http_exception_handler(request: Request, exc: HTTPException):
    logger.error("HTTP exception", path=request.url.path, status_code=exc.status_code, detail=exc.detail)
    return JSONResponse(
        status_code=exc.status_code,
        content={"error": exc.detail, "status_code": exc.status_code}
    )

@app.exception_handler(Exception)
async def general_exception_handler(request: Request, exc: Exception):
    logger.error("Unhandled exception", path=request.url.path, error=str(exc), type=type(exc).__name__)
    return JSONResponse(
        status_code=500,
        content={"error": "Internal server error", "status_code": 500}
    )

if __name__ == "__main__":
    uvicorn.run(
        "main:app",
        host=config.HOST,
        port=config.PORT,
        reload=False,
        log_config={
            "version": 1,
            "disable_existing_loggers": False,
            "formatters": {
                "default": {
                    "()": "uvicorn.logging.DefaultFormatter",
                    "fmt": "%(asctime)s - %(name)s - %(levelname)s - %(message)s",
                },
            },
            "handlers": {
                "default": {
                    "formatter": "default",
                    "class": "logging.StreamHandler",
                    "stream": "ext://sys.stdout",
                },
            },
            "root": {
                "level": "INFO",
                "handlers": ["default"],
            },
        }
    )