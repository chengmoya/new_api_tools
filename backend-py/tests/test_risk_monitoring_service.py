import importlib
import sys
import types
from pathlib import Path

import pytest


PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))


if "app.database" not in sys.modules:
    database_stub = types.ModuleType("app.database")

    class DatabaseEngine:
        POSTGRESQL = "postgresql"
        MYSQL = "mysql"

    class DBConfig:
        pass

    class DatabaseManager:
        pass

    def get_db_manager():
        return None

    def reset_db_manager():
        return None

    database_stub.DatabaseEngine = DatabaseEngine
    database_stub.DBConfig = DBConfig
    database_stub.DatabaseManager = DatabaseManager
    database_stub.get_db_manager = get_db_manager
    database_stub.reset_db_manager = reset_db_manager
    sys.modules["app.database"] = database_stub

if "app.logger" not in sys.modules:
    logger_stub = types.ModuleType("app.logger")

    class DummyLogger:
        def __getattr__(self, _name):
            return lambda *args, **kwargs: None

    logger_stub.logger = DummyLogger()
    sys.modules["app.logger"] = logger_stub


risk_monitoring_service = importlib.import_module("app.risk_monitoring_service")
RiskMonitoringService = risk_monitoring_service.RiskMonitoringService


class FakeDB:
    def __init__(self, handlers):
        self.handlers = handlers
        self.config = types.SimpleNamespace(engine="mysql")

    def connect(self):
        return None

    def execute(self, sql, params):
        normalized_sql = " ".join(sql.split())
        for matcher, handler in self.handlers:
            if matcher in normalized_sql:
                return handler(params)
        raise AssertionError(f"Unexpected SQL: {normalized_sql}")


@pytest.fixture(autouse=True)
def clear_cache():
    risk_monitoring_service._cache.clear()
    yield
    risk_monitoring_service._cache.clear()


def test_get_low_token_burst_users_returns_expected_metrics(monkeypatch):
    def summary_handler(_params):
        return [
            {
                "user_id": 101,
                "username": "burst-user",
                "total_requests": 80,
                "low_token_requests": 72,
                "avg_tokens_per_request": 188.6,
                "max_tokens_per_request": 460,
                "min_tokens_per_request": 12,
                "first_seen": 1700000000,
                "last_seen": 1700001800,
            }
        ]

    service = RiskMonitoringService(db=FakeDB([
        ("SUM( CASE WHEN (COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0)) <= :low_token_threshold", summary_handler),
    ]))
    monkeypatch.setattr(risk_monitoring_service, "_get_cache_ttl", lambda: 60)

    result = service.get_low_token_burst_users(
        window_seconds=3600,
        low_token_threshold=500,
        low_token_ratio_threshold=0.8,
        low_token_request_threshold=50,
        use_cache=False,
    )

    assert result["total"] == 1
    assert result["thresholds"]["low_token_threshold"] == 500
    item = result["items"][0]
    assert item["user_id"] == 101
    assert item["rule_name"] == "LOW_TOKEN_BURST"
    assert item["low_token_requests"] == 72
    assert item["low_token_ratio"] == pytest.approx(0.9)
    assert item["risk_level"] == "high"


def test_get_token_usage_volatility_users_detects_jump_ratio(monkeypatch):
    def logs_handler(_params):
        return [
            {
                "user_id": 202,
                "username": "volatile-user",
                "token_id": 9001,
                "token_name": "primary-token",
                "created_at": 1700000000,
                "total_tokens": 400,
            },
            {
                "user_id": 202,
                "username": "volatile-user",
                "token_id": 9001,
                "token_name": "primary-token",
                "created_at": 1700000030,
                "total_tokens": 20000,
            },
            {
                "user_id": 202,
                "username": "volatile-user",
                "token_id": 9001,
                "token_name": "primary-token",
                "created_at": 1700000060,
                "total_tokens": 3000,
            },
            {
                "user_id": 202,
                "username": "volatile-user",
                "token_id": 9001,
                "token_name": "primary-token",
                "created_at": 1700000090,
                "total_tokens": 500,
            },
            {
                "user_id": 202,
                "username": "volatile-user",
                "token_id": 9001,
                "token_name": "primary-token",
                "created_at": 1700000120,
                "total_tokens": 450,
            },
        ]

    service = RiskMonitoringService(db=FakeDB([
        ("COALESCE(prompt_tokens, 0) + COALESCE(completion_tokens, 0) as total_tokens", logs_handler),
    ]))
    monkeypatch.setattr(risk_monitoring_service, "_get_cache_ttl", lambda: 60)

    result = service.get_token_usage_volatility_users(
        window_seconds=3600,
        min_requests=5,
        jump_ratio=5.0,
        use_cache=False,
    )

    assert result["total"] == 1
    item = result["items"][0]
    assert item["user_id"] == 202
    assert item["rule_name"] == "TOKEN_USAGE_VOLATILITY"
    assert item["token_count"] == 1
    assert item["max_adjacent_jump_ratio"] == pytest.approx(50.0)
    assert item["risk_level"] == "high"
    suspicious_token = item["suspicious_tokens"][0]
    assert suspicious_token["largest_jump_pair"]["from_tokens"] == 400
    assert suspicious_token["largest_jump_pair"]["to_tokens"] == 20000
    assert suspicious_token["sample_values"] == [400, 20000, 3000, 500, 450]
