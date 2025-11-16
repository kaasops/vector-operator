#!/usr/bin/env python3
"""
Generate HTML Pivot Grid Report for E2E Test Results (Enhanced V2)

Features:
- Interactive pivot grid showing test results across multiple runs
- Trend analysis charts (Pass Rate, Duration)
- Advanced Log Viewer with ANSI support and filtering
- Deep Flakiness Analysis (Score, Patterns)
- Run Comparison (New Failures, Fixed Tests)
- Smart artifact matching
- Filtering and Search
"""

import json
import sys
import html
import re
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Any, Optional, Set
from dataclasses import dataclass, field

# --- SVG Icons ---

def svg_icon(name: str, size: int = 16, color: str = 'currentColor') -> str:
    """Generate inline SVG icons"""
    icons = {
        'search': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.35-4.35"></path></svg>',
        'copy': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>',
        'download': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>',
        'error': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>',
        'warning': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>',
        'info': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="16" x2="12" y2="12"></line><line x1="12" y1="8" x2="12.01" y2="8"></line></svg>',
        'bug': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m8 2 1.88 1.88"></path><path d="M14.12 3.88 16 2"></path><path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1"></path><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6"></path><path d="M12 20v-9"></path><path d="M6.53 9C4.6 8.8 3 7.1 3 5"></path><path d="M6 13H2"></path><path d="M3 21c0-2.1 1.7-3.9 3.8-4"></path><path d="M20.97 5c0 2.1-1.6 3.8-3.5 4"></path><path d="M22 13h-4"></path><path d="M17.2 17c2.1.1 3.8 1.9 3.8 4"></path></svg>',
        'chevron-up': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="18 15 12 9 6 15"></polyline></svg>',
        'chevron-down': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>',
        'arrow-up': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"></line><polyline points="5 12 12 5 19 12"></polyline></svg>',
        'arrow-down': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><polyline points="19 12 12 19 5 12"></polyline></svg>',
        'wrap': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"></polyline><path d="M3 11V9a4 4 0 0 1 4-4h14"></path><polyline points="7 23 3 19 7 15"></polyline><path d="M21 13v2a4 4 0 0 1-4 4H3"></path></svg>',
        'sun': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>',
        'moon': f'<svg width="{size}" height="{size}" viewBox="0 0 24 24" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>',
    }
    return icons.get(name, '')

def format_duration(seconds: float) -> str:
    """Format duration in human-readable format"""
    if not seconds or seconds < 0:
        return 'N/A'

    hours = int(seconds // 3600)
    minutes = int((seconds % 3600) // 60)
    secs = int(seconds % 60)

    if hours > 0:
        return f"{hours}h {minutes}m {secs}s"
    elif minutes > 0:
        return f"{minutes}m {secs}s"
    else:
        return f"{secs}s"

# --- Data Structures ---

@dataclass
class TestResult:
    name: str
    full_name: str
    leaf_text: str
    state: str
    runtime: float
    failure_message: str = ""
    labels: List[str] = field(default_factory=list)
    container_hierarchy: List[str] = field(default_factory=list)
    start_time: str = ""
    end_time: str = ""
    artifact_metadata: Optional[Dict[str, Any]] = None

@dataclass
class TestRun:
    run_id: str
    start_time: str
    total_tests: int
    passed_tests: int
    failed_tests: int
    environment: Dict[str, Any]
    total_runtime: float
    test_output_log: str
    tests: List[TestResult]
    git_commit: str = ""
    git_branch: str = ""
    git_dirty: str = ""
    description: str = ""

    @property
    def date_str(self) -> str:
        return datetime.fromisoformat(self.start_time).strftime('%Y-%m-%d %H:%M')

@dataclass
class PivotRow:
    test_name: str
    full_test_name: str
    leaf_text: str
    container_hierarchy: List[str]
    runs: Dict[str, Any] = field(default_factory=dict) # run_id -> result dict
    
    # Stats
    total_runs: int = 0
    pass_count: int = 0
    fail_count: int = 0
    skip_count: int = 0
    pass_rate: float = 0.0
    total_runtime: float = 0.0
    avg_runtime: float = 0.0
    min_runtime: float = float('inf')
    max_runtime: float = 0.0
    
    # Flakiness
    is_flaky: bool = False
    flakiness_score: float = 0.0
    flakiness_pattern: str = 'stable'

# --- Templates ---

class ReportTemplates:
    """Holds HTML, CSS, and JS templates."""

    CSS = """
        :root {
            /* Light theme colors */
            --bg-primary: #ffffff;
            --bg-secondary: #f8fafc;
            --bg-tertiary: #fafbfc;
            --bg-hover: #e0e7ff;
            --cell-hover-bg: #eff6ff;
            --cell-active-bg: #dbeafe;
            --text-primary: #0f172a;
            --text-secondary: #64748b;
            --text-tertiary: #94a3b8;
            --border-color: #e2e8f0;
            --border-secondary: #cbd5e1;
            --modal-backdrop: rgba(0,0,0,0.4);
            --accent-color: #3b82f6;

            /* Status colors */
            --success-bg: #d1fae5;
            --success-text: #065f46;
            --error-bg: #fee2e2;
            --error-text: #991b1b;
            --warning-bg: #fef3c7;
            --warning-text: #92400e;
            --info-bg: #dbeafe;
            --info-text: #1e40af;

            /* Log viewer colors */
            --log-bg: #1e293b;
            --log-text: #e2e8f0;
            --log-border: #334155;
            --log-controls-bg: #334155;
            --log-input-bg: #1e293b;
            --log-input-border: #475569;
        }

        [data-theme="dark"] {
            /* Dark theme colors */
            --bg-primary: #1e293b;
            --bg-secondary: #0f172a;
            --bg-tertiary: #1e293b;
            --bg-hover: #334155;
            --cell-hover-bg: rgba(59, 130, 246, 0.2);
            --cell-active-bg: rgba(59, 130, 246, 0.3);
            --text-primary: #f1f5f9;
            --text-secondary: #cbd5e1;
            --text-tertiary: #94a3b8;
            --border-color: #334155;
            --border-secondary: #475569;
            --modal-backdrop: rgba(0,0,0,0.7);
            --accent-color: #60a5fa;

            /* Status colors */
            --success-bg: #064e3b;
            --success-text: #a7f3d0;
            --error-bg: #7f1d1d;
            --error-text: #fca5a5;
            --warning-bg: #78350f;
            --warning-text: #fcd34d;
            --info-bg: #1e3a8a;
            --info-text: #93c5fd;

            /* Log viewer colors */
            --log-bg: #0f172a;
            --log-text: #e2e8f0;
            --log-border: #1e293b;
        }

        /* Row hover effect */
        tbody tr {
            transition: background-color 0.15s ease;
        }
        tbody tr:hover {
            background-color: var(--bg-hover);
        }
        tbody tr:hover .test-name-cell,
        tbody tr:hover .stats-cell {
            background-color: var(--bg-hover);
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-secondary);
            color: var(--text-primary);
            min-height: 100vh;
        }

        /* SVG Icons */
        svg {
            display: inline-block;
            vertical-align: middle;
            flex-shrink: 0;
        }
        button svg, .filter-label svg, .log-actions-item svg {
            margin-right: 6px;
        }

        .container { background: var(--bg-primary); min-height: 100vh; display: flex; flex-direction: column; }
        
        /* Header & Tabs */
        .header {
            padding: 20px 40px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            justify-content: space-between;
            align-items: center;
            background: var(--bg-primary);
        }
        .header h1 { font-size: 22px; font-weight: 600; margin-bottom: 4px; }
        
        .tabs {
            display: flex;
            padding: 0 40px;
            background: var(--bg-primary);
            border-bottom: 1px solid var(--border-color);
            gap: 24px;
        }
        .tab-btn {
            padding: 16px 4px;
            background: none;
            border: none;
            border-bottom: 2px solid transparent;
            color: var(--text-secondary);
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s;
        }
        .tab-btn:hover { color: var(--text-primary); }
        .tab-btn.active {
            color: var(--accent-color);
            border-bottom-color: var(--accent-color);
        }

        .tab-content { display: none; padding: 24px 40px; }
        .tab-content.active { display: block; }

        /* Summary Cards */
        .summary-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }
        .card {
            background: var(--bg-primary);
            padding: 20px;
            border-radius: 8px;
            border: 1px solid var(--border-color);
            text-align: center;
            display: flex;
            flex-direction: column;
            justify-content: center;
            min-height: 100px;
        }
        .card .label { font-size: 13px; color: var(--text-secondary); text-transform: uppercase; font-weight: 600; margin-bottom: 8px; }
        .card .value { font-size: 32px; font-weight: 700; color: var(--text-primary); text-align: center; display: block; }
        .card.passed .value { color: #10b981; }
        .card.failed .value { color: #ef4444; }

        /* Tooltips */
        .tooltip {
            position: relative;
            cursor: help;
        }
        .card.tooltip {
            display: flex;
            flex-direction: column;
            justify-content: center;
            min-height: 100px;
        }
        .pass-rate.tooltip {
            display: inline-block;
        }
        .badge.tooltip {
            position: relative;
            display: inline-block;
        }
        .tooltip .tooltiptext {
            visibility: hidden;
            width: 250px;
            background-color: #1f2937;
            color: #fff;
            text-align: left;
            border-radius: 6px;
            padding: 8px 12px;
            position: absolute;
            z-index: 1000;
            top: 100%;
            margin-top: 8px;
            left: 50%;
            margin-left: -125px;
            opacity: 0;
            transition: opacity 0.3s;
            font-size: 12px;
            line-height: 1.4;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
            pointer-events: none;
        }
        .tooltip .tooltiptext::after {
            content: "";
            position: absolute;
            bottom: 100%;
            left: 50%;
            margin-left: -5px;
            border-width: 5px;
            border-style: solid;
            border-color: transparent transparent #1f2937 transparent;
        }
        .tooltip:hover .tooltiptext {
            visibility: visible;
            opacity: 1;
        }

        /* Charts */
        .charts-container {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 24px;
            margin-bottom: 32px;
        }
        .chart-wrapper {
            background: var(--bg-primary);
            padding: 20px;
            border-radius: 12px;
            border: 1px solid var(--border-color);
            height: 350px;
        }
        
        /* Flaky Section */
        .flaky-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 16px;
            margin-top: 16px;
        }
        .flaky-card {
            text-align: left;
            cursor: pointer;
            transition: transform 0.2s;
        }
        .flaky-card:hover { transform: translateY(-2px); border-color: var(--accent-color); }
        .flaky-header { display: flex; justify-content: space-between; margin-bottom: 8px; }
        .flaky-name { font-weight: 500; margin-bottom: 8px; font-size: 14px; overflow: hidden; text-overflow: ellipsis; }
        .flaky-stats { font-size: 12px; color: var(--text-secondary); }

        /* Table Styles */
        .table-wrapper {
            overflow-x: auto;
            border: 1px solid var(--border-color);
            border-radius: 8px;
        }
        table { width: 100%; border-collapse: collapse; font-size: 14px; }
        th, td {
            padding: 12px 16px;
            border-bottom: 1px solid var(--border-color);
            text-align: left;
            white-space: nowrap;
        }
        th {
            background: var(--bg-secondary);
            font-weight: 600;
            color: var(--text-secondary);
            position: sticky;
            top: 0;
            z-index: 10;
        }
        th.run-header {
            cursor: pointer;
            transition: all 0.2s ease;
        }
        th.run-header:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
            transform: scale(1.02);
        }
        th:first-child, td:first-child {
            position: sticky;
            left: 0;
            background: var(--bg-primary);
            z-index: 11;
            border-right: 1px solid var(--border-color);
            min-width: 500px;
            max-width: 700px;
            white-space: normal;
        }
        th:first-child { background: var(--bg-secondary); z-index: 12; }

        /* Breadcrumb Test Names */
        .test-name-cell {
            padding: 10px 12px !important;
            line-height: 1.5;
            position: relative;
        }
        .test-name-wrapper {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .copy-test-name-btn {
            opacity: 0;
            transition: opacity 0.2s ease;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            border-radius: 4px;
            padding: 4px 8px;
            cursor: pointer;
            font-size: 11px;
            color: var(--text-secondary);
            display: flex;
            align-items: center;
            gap: 4px;
            white-space: nowrap;
        }
        .copy-test-name-btn:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
            border-color: var(--text-tertiary);
        }
        .copy-test-name-btn:active {
            transform: scale(0.95);
        }
        tbody tr:hover .copy-test-name-btn {
            opacity: 1;
        }
        .test-breadcrumb {
            display: flex;
            align-items: center;
            flex-wrap: wrap;
            gap: 6px;
            font-size: 13px;
        }
        .breadcrumb-item {
            display: inline-flex;
            align-items: center;
        }
        .breadcrumb-container {
            color: var(--text-secondary);
            font-weight: 500;
        }
        .breadcrumb-container.level-0 {
            color: var(--text-primary);
            font-weight: 600;
        }
        .breadcrumb-separator {
            color: var(--text-tertiary);
            margin: 0 6px;
            font-weight: 300;
            user-select: none;
        }
        .breadcrumb-leaf {
            color: var(--text-primary);
            font-weight: 400;
        }
        
        .result-cell {
            text-align: center;
            cursor: pointer !important;
            transition: all 0.2s ease;
            position: relative;
        }
        .result-cell:hover {
            background: var(--cell-hover-bg) !important;
            transform: translateY(-1px);
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .result-cell:active {
            transform: translateY(0);
        }
        .result-cell * {
            cursor: pointer !important;
            pointer-events: none;
        }
        .badge {
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
            pointer-events: none;
        }
        .badge.passed { background: var(--success-bg); color: var(--success-text); }
        .badge.failed { background: var(--error-bg); color: var(--error-text); }
        .badge.skipped { background: var(--warning-bg); color: var(--warning-text); }

        /* Runtime Badges */
        .runtime { margin-left: 6px; font-size: 11px; pointer-events: none; }
        .runtime-fast { color: #10b981; }
        .runtime-medium { color: #f59e0b; }
        .runtime-slow { color: #ef4444; }

        /* Stats Column */
        .stats-col, .stats-cell {
            position: sticky;
            left: 500px; /* after test name */
            background: var(--bg-primary);
            border-right: 1px solid var(--border-color);
            min-width: 120px;
            text-align: center;
            font-size: 12px;
            z-index: 11;
        }
        .stats-col { z-index: 12; background: var(--bg-secondary); }
        .pass-rate { padding: 4px 8px; border-radius: 4px; margin-bottom: 4px; display: inline-block; font-weight: bold; }
        .rate-high { background: var(--success-bg); color: var(--success-text); }
        .rate-medium { background: var(--warning-bg); color: var(--warning-text); }
        .rate-low { background: var(--error-bg); color: var(--error-text); }
        .counts { color: var(--text-secondary); margin-bottom: 2px; }
        .avg-time { color: var(--text-tertiary); font-size: 10px; }

        /* Filters */
        .filters {
            display: flex;
            gap: 16px;
            margin-bottom: 20px;
            flex-wrap: wrap;
        }
        .filter-input {
            padding: 8px 12px;
            border: 1px solid var(--border-secondary);
            border-radius: 6px;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-width: 200px;
        }

        /* Modal */
        .modal {
            display: none;
            position: fixed;
            top: 0; left: 0; width: 100%; height: 100%;
            background: var(--modal-backdrop);
            z-index: 1000;
            backdrop-filter: blur(2px);
        }
        .modal-content {
            background: var(--bg-primary);
            width: 90%; max-width: 1200px;
            margin: 20px auto 30px;
            border-radius: 12px;
            max-height: calc(100vh - 50px);
            display: flex;
            flex-direction: column;
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
        }
        .modal-header {
            padding: 20px 30px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-shrink: 0;
        }
        .modal-header h2 {
            font-size: 18px;
            margin: 0;
        }
        .modal-body {
            padding: 0;
            overflow-y: auto;
            background: var(--bg-secondary);
            flex: 1;
        }
        .close-btn {
            font-size: 28px;
            cursor: pointer;
            color: var(--text-secondary);
            transition: color 0.2s;
        }
        .close-btn:hover {
            color: var(--text-primary);
        }

        /* Modal Tabs */
        .modal-tabs {
            display: flex;
            background: var(--bg-primary);
            border-bottom: 1px solid var(--border-color);
            padding: 0 30px;
            gap: 24px;
        }
        .modal-tab {
            padding: 14px 4px;
            background: none;
            border: none;
            border-bottom: 2px solid transparent;
            color: var(--text-secondary);
            font-weight: 500;
            font-size: 14px;
            cursor: pointer;
            transition: all 0.2s;
        }
        .modal-tab:hover {
            color: var(--text-primary);
        }
        .modal-tab.active {
            color: var(--accent-color);
            border-bottom-color: var(--accent-color);
        }
        .modal-tab-content {
            display: none;
            padding: 30px;
        }
        .modal-tab-content.active {
            display: block;
        }

        /* Test Detail Sections */
        .test-detail-section {
            background: var(--bg-primary);
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 16px;
            border: 1px solid var(--border-color);
        }
        .test-detail-section h4 {
            margin: 0 0 16px 0;
            font-size: 14px;
            font-weight: 600;
            color: var(--text-primary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .detail-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 16px;
        }
        .detail-item {
            display: flex;
            flex-direction: column;
        }
        .detail-label {
            font-size: 11px;
            color: var(--text-tertiary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 4px;
        }
        .detail-value {
            font-size: 14px;
            color: var(--text-primary);
            font-weight: 500;
        }
        .detail-value code {
            background: var(--bg-secondary);
            padding: 2px 6px;
            border-radius: 4px;
            font-size: 12px;
        }

        /* Artifact List */
        .artifact-list {
            display: flex;
            flex-direction: column;
            gap: 12px;
        }
        .artifact-item {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 16px;
        }
        .artifact-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }
        .artifact-name {
            font-weight: 600;
            font-size: 14px;
            color: var(--text-primary);
        }
        .artifact-meta {
            font-size: 11px;
            color: var(--text-tertiary);
        }

        /* Error Box */
        .error-box {
            background: var(--error-bg);
            border: 1px solid var(--error-text);
            border-radius: 8px;
            padding: 16px;
            margin-top: 16px;
        }
        .error-box h4 {
            margin: 0 0 12px 0;
            color: var(--error-text);
            font-size: 14px;
        }
        .error-box pre {
            margin: 0;
            color: var(--error-text);
            font-size: 12px;
            white-space: pre-wrap;
            word-break: break-word;
        }
        
        /* Log Viewer */
        .log-viewer {
            background: var(--log-bg);
            color: var(--log-text);
            padding: 0;
            border-radius: 8px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 13px;
            max-height: 600px;
            overflow-y: auto;
            line-height: 1.6;
            counter-reset: line-number;
        }
        .log-viewer.with-line-numbers {
            padding-left: 0;
        }
        .log-line {
            display: flex;
            padding: 2px 0;
            position: relative;
        }
        .log-line:hover {
            background: rgba(59, 130, 246, 0.15);
        }
        .log-line-number {
            counter-increment: line-number;
            flex-shrink: 0;
            width: 50px;
            padding: 0 12px;
            text-align: right;
            color: var(--text-tertiary);
            user-select: none;
            border-right: 1px solid var(--border-color);
            font-size: 11px;
            line-height: 1.6;
        }
        .log-line-number::before {
            content: counter(line-number);
        }
        .log-line-number:hover {
            color: var(--text-secondary);
            cursor: pointer;
        }
        .log-line-content {
            flex: 1;
            padding: 0 12px;
            white-space: pre-wrap;
            word-break: break-word;
        }

        /* Log syntax highlighting */
        .log-viewer .log-passed {
            color: #10b981;
            font-weight: 600;
        }
        .log-viewer .log-failed {
            color: #ef4444;
            font-weight: 600;
            background: rgba(239, 68, 68, 0.1);
            padding: 2px 4px;
            border-radius: 2px;
        }
        .log-viewer .log-step {
            color: #3b82f6;
            font-weight: 600;
        }
        .log-viewer .log-error {
            color: #f97316;
            font-weight: 600;
            background: rgba(249, 115, 22, 0.1);
            padding: 2px 4px;
            border-radius: 2px;
        }
        .log-viewer .log-timestamp {
            color: #6366f1;
            opacity: 0.8;
            font-size: 0.95em;
        }
        .log-viewer .log-duration {
            color: #8b5cf6;
            font-weight: 500;
        }
        .log-viewer .log-command {
            color: #06b6d4;
            font-style: italic;
        }
        .log-viewer .log-test-name {
            color: #fbbf24;
            font-weight: 500;
        }
        .log-viewer .log-warning {
            color: #f59e0b;
            font-weight: 500;
        }

        /* Test separators in logs */
        .log-test-separator {
            border-top: 2px solid var(--border-color);
            margin: 20px 0 16px 0;
            padding-top: 16px;
            position: relative;
        }
        .log-test-separator::before {
            content: '━━━━';
            position: absolute;
            top: -13px;
            left: 0;
            background: var(--log-bg);
            padding-right: 10px;
            color: var(--border-color);
            font-size: 14px;
            letter-spacing: 2px;
        }
        .log-test-header {
            font-weight: 600;
            color: var(--accent-color);
            font-size: 14px;
            margin-bottom: 8px;
            padding: 8px 12px;
            background: var(--bg-hover);
            border-left: 3px solid var(--accent-color);
            border-radius: 4px;
        }
        .log-test-header .test-status {
            float: right;
            font-size: 12px;
            padding: 2px 8px;
            border-radius: 3px;
            font-weight: 600;
        }
        .log-test-header .test-status.passed {
            background: #d1fae5;
            color: #065f46;
        }
        .log-test-header .test-status.failed {
            background: #fee2e2;
            color: #991b1b;
        }

        .log-controls {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 12px;
            padding: 10px 12px;
            background: linear-gradient(to bottom, var(--bg-primary), var(--bg-hover));
            border: 1px solid var(--border-color);
            border-radius: 8px;
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
            flex-wrap: wrap;
        }
        .log-controls > strong {
            margin-right: 8px;
            font-size: 13px;
            color: var(--text-primary);
        }
        .toolbar-separator {
            width: 1px;
            height: 24px;
            background: var(--border-color);
            margin: 0 4px;
        }
        .log-btn {
            padding: 4px 12px;
            background: var(--log-controls-bg);
            border: 1px solid var(--log-border);
            color: var(--log-text);
            border-radius: 4px;
            cursor: pointer;
        }
        .log-btn.active { background: var(--accent-color); color: white; }

        /* Control groups */
        .log-actions-menu,
        .log-filters-menu {
            position: relative;
            display: inline-flex;
            margin-right: 6px;
        }
        .log-search-controls {
            display: flex;
            align-items: center;
            gap: 4px;
            padding: 4px 8px;
            background: var(--bg-hover);
            border-radius: 6px;
        }
        .log-nav-buttons {
            display: flex;
            gap: 4px;
            padding: 4px 8px;
            background: var(--bg-hover);
            border-radius: 6px;
        }
        .log-display-controls {
            display: flex;
            gap: 6px;
            align-items: center;
            padding: 4px 8px;
            background: var(--bg-hover);
            border-radius: 6px;
        }
        .log-search-input {
            padding: 4px 8px;
            border: 1px solid var(--border-color);
            border-radius: 4px;
            font-size: 11px;
            width: 180px;
            outline: none;
            transition: all 0.2s ease;
        }
        .log-search-input:focus {
            border-color: var(--accent-color);
            box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
        }
        .log-search-btn {
            padding: 4px 8px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 4px;
            cursor: pointer;
            font-size: 13px;
            transition: all 0.2s ease;
            min-width: 28px;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .log-search-btn:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
            border-color: var(--accent-color);
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
            transform: translateY(-1px);
        }
        .log-search-btn:active {
            transform: translateY(0);
            box-shadow: none;
        }
        .log-search-btn:disabled {
            opacity: 0.4;
            cursor: not-allowed;
        }
        .search-counter {
            font-size: 11px;
            color: var(--text-secondary);
            min-width: 40px;
            text-align: center;
        }
        /* Search result highlighting */
        .search-highlight {
            background: #bfdbfe;
            color: #1e3a8a;
            border-radius: 2px;
            padding: 0 2px;
        }
        .theme-dark .search-highlight {
            background: #1e3a8a;
            color: #bfdbfe;
        }
        .search-highlight-current {
            background: #3b82f6;
            color: white;
            border-radius: 2px;
            padding: 0 2px;
            font-weight: 600;
            box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.3);
        }
        .theme-dark .search-highlight-current {
            background: #60a5fa;
            color: #0f172a;
        }

        /* Actions and Filters menus */
        .log-actions-btn {
            padding: 4px 8px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 4px;
            cursor: pointer;
            font-size: 16px;
            transition: all 0.2s ease;
            min-width: 32px;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .log-actions-btn:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
            border-color: var(--accent-color);
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
            transform: translateY(-1px);
        }
        .log-actions-btn:active {
            transform: translateY(0);
            box-shadow: none;
        }
        .log-actions-dropdown {
            display: none;
            position: absolute;
            top: 100%;
            left: 0;
            margin-top: 4px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            z-index: 1000;
            min-width: 160px;
            overflow: hidden;
        }
        .log-actions-dropdown.show {
            display: block;
        }
        .log-actions-item {
            padding: 8px 12px;
            cursor: pointer;
            font-size: 12px;
            color: var(--text-primary);
            transition: background 0.15s ease;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .log-actions-item:hover {
            background: var(--bg-hover);
        }
        .log-actions-divider {
            height: 1px;
            background: var(--border-color);
            margin: 4px 0;
        }

        /* Filters menu */
        .log-filters-btn {
            padding: 4px 10px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 4px;
            cursor: pointer;
            font-size: 11px;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            gap: 4px;
        }
        .log-filters-btn:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
            border-color: var(--accent-color);
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
            transform: translateY(-1px);
        }
        .log-filters-btn:active {
            transform: translateY(0);
            box-shadow: none;
        }
        .log-filters-dropdown {
            display: none;
            position: absolute;
            top: 100%;
            left: 0;
            margin-top: 4px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            z-index: 1000;
            min-width: 180px;
            padding: 8px 0;
            overflow: hidden;
        }
        .log-filters-dropdown.show {
            display: block;
        }
        .log-filters-header {
            padding: 4px 12px 8px;
            font-size: 11px;
            font-weight: 600;
            color: var(--text-secondary);
            text-transform: uppercase;
            border-bottom: 1px solid var(--border-color);
            margin-bottom: 4px;
        }
        .log-filter-item {
            padding: 6px 12px;
            cursor: pointer;
            font-size: 12px;
            color: var(--text-primary);
            transition: background 0.15s ease;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .log-filter-item:hover {
            background: var(--bg-hover);
        }
        .log-filter-item input[type="checkbox"] {
            margin: 0;
            cursor: pointer;
        }
        .filter-label {
            display: flex;
            align-items: center;
            gap: 6px;
            flex: 1;
        }
        .filter-count {
            margin-left: auto;
            font-size: 11px;
            color: var(--text-secondary);
            background: var(--bg-hover);
            padding: 2px 6px;
            border-radius: 10px;
        }
        .log-filters-actions {
            display: flex;
            gap: 6px;
            padding: 8px 12px 4px;
            border-top: 1px solid var(--border-color);
            margin-top: 4px;
        }
        .log-filter-action-btn {
            flex: 1;
            padding: 4px 8px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 4px;
            cursor: pointer;
            font-size: 11px;
            transition: all 0.2s ease;
        }
        .log-filter-action-btn:hover {
            background: var(--bg-hover);
            border-color: var(--accent-color);
        }
        .log-filter-action-btn.primary {
            background: var(--accent-color);
            color: white;
            border-color: var(--accent-color);
        }
        .log-filter-action-btn.primary:hover {
            background: #2563eb;
        }

        /* Navigation buttons */
        .log-nav-btn {
            padding: 4px 10px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 4px;
            cursor: pointer;
            font-size: 11px;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            gap: 4px;
        }
        .log-nav-btn:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
            border-color: var(--accent-color);
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
            transform: translateY(-1px);
        }
        .log-nav-btn:active {
            transform: translateY(0);
            box-shadow: none;
        }
        .log-nav-btn:disabled {
            opacity: 0.4;
            cursor: not-allowed;
        }
        .log-nav-btn:disabled:hover {
            background: var(--bg-primary);
            color: var(--text-secondary);
            border-color: var(--border-color);
        }
        .log-nav-btn .nav-icon {
            font-size: 12px;
        }
        .log-nav-btn.error-nav {
            border-color: #ef4444;
            color: #ef4444;
        }
        .log-nav-btn.error-nav:hover {
            background: #fef2f2;
            box-shadow: 0 1px 3px rgba(239, 68, 68, 0.2);
            transform: translateY(-1px);
        }

        /* Display controls */
        .log-zoom-control {
            display: flex;
            gap: 2px;
            align-items: center;
        }
        .log-zoom-btn {
            padding: 2px 6px;
            background: var(--bg-primary);
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            border-radius: 3px;
            cursor: pointer;
            font-size: 12px;
            font-weight: 600;
            transition: all 0.2s ease;
        }
        .log-zoom-btn:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
            border-color: var(--accent-color);
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
            transform: scale(1.05);
        }
        .log-zoom-btn:active {
            transform: scale(0.95);
            box-shadow: none;
        }
        .log-zoom-value {
            font-size: 11px;
            color: var(--text-tertiary);
            min-width: 35px;
            text-align: center;
        }
        .log-viewer.font-sm { font-size: 11px; }
        .log-viewer.font-md { font-size: 13px; }
        .log-viewer.font-lg { font-size: 15px; }
        .log-viewer.font-xl { font-size: 17px; }
        .log-viewer.wrap-enabled .log-line-content {
            white-space: pre-wrap;
            word-break: break-word;
        }
        .log-viewer.wrap-disabled .log-line-content {
            white-space: pre;
            overflow-x: auto;
        }

        /* Floating Controls Panel (Google Maps style) */
        .log-floating-controls {
            position: absolute;
            bottom: 16px;
            right: 16px;
            display: flex;
            flex-direction: column;
            gap: 10px;
            z-index: 100;
            pointer-events: none;
            opacity: 0.3;
            transition: opacity 0.2s ease;
        }
        .log-floating-controls:hover {
            opacity: 1;
        }
        .log-floating-controls > * {
            pointer-events: auto;
        }
        .floating-control-group {
            background: rgba(255, 255, 255, 0.95);
            border: none;
            border-radius: 2px;
            padding: 4px;
            box-shadow: rgba(0, 0, 0, 0.3) 0px 1px 4px -1px;
            display: flex;
            flex-direction: column;
            gap: 0;
            align-items: center;
        }
        .theme-dark .floating-control-group {
            background: rgba(30, 41, 59, 0.75);
            box-shadow: rgba(0, 0, 0, 0.4) 0px 2px 6px 0px;
            backdrop-filter: blur(8px);
        }
        .floating-control-divider {
            width: 20px;
            height: 1px;
            background: #e5e7eb;
            margin: 4px 0;
        }
        .theme-dark .floating-control-divider {
            background: #4a5568;
        }
        .floating-zoom-control {
            display: flex;
            flex-direction: column;
            gap: 0;
            align-items: center;
        }
        .floating-zoom-btn {
            width: 28px;
            height: 28px;
            padding: 0;
            background: transparent;
            border: none;
            color: #5f6368;
            border-radius: 2px;
            cursor: pointer;
            font-size: 18px;
            font-weight: 400;
            line-height: 1;
            transition: background 0.1s ease;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .floating-zoom-btn:hover {
            background: #f1f3f4;
        }
        .theme-dark .floating-zoom-btn {
            color: #e2e8f0;
        }
        .theme-dark .floating-zoom-btn:hover {
            background: #4a5568;
        }
        .floating-zoom-btn:active {
            background: #e8eaed;
        }
        .theme-dark .floating-zoom-btn:active {
            background: #2d3748;
        }
        .floating-zoom-value {
            font-size: 10px;
            color: #5f6368;
            font-weight: 400;
            text-align: center;
            min-width: 28px;
            padding: 2px 0;
        }
        .theme-dark .floating-zoom-value {
            color: #cbd5e0;
        }
        .floating-wrap-btn {
            width: 28px;
            height: 28px;
            padding: 0;
            background: transparent;
            border: none;
            color: #5f6368;
            border-radius: 2px;
            cursor: pointer;
            font-size: 14px;
            line-height: 1;
            transition: background 0.1s ease;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .floating-wrap-btn:hover {
            background: #f1f3f4;
        }
        .theme-dark .floating-wrap-btn {
            color: #e2e8f0;
        }
        .theme-dark .floating-wrap-btn:hover {
            background: #4a5568;
        }
        .floating-wrap-btn:active {
            background: #e8eaed;
        }
        .theme-dark .floating-wrap-btn:active {
            background: #2d3748;
        }
        .floating-wrap-btn.active {
            background: #e8f0fe;
            color: #1a73e8;
        }
        .theme-dark .floating-wrap-btn.active {
            background: #3b82f6;
            color: white;
        }
        .log-line-number-clickable {
            position: relative;
            cursor: pointer;
        }
        .log-line-number-clickable:hover {
            background: rgba(59, 130, 246, 0.1);
        }

        /* pprof Visualization - Compact */
        .pprof-compact-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 12px;
            padding: 8px 12px;
            background: var(--bg-secondary);
            border-radius: 6px;
            flex-wrap: wrap;
            gap: 8px;
        }
        .pprof-stats {
            display: flex;
            align-items: center;
            gap: 6px;
            font-size: 13px;
            color: var(--text-primary);
            flex-wrap: wrap;
        }
        .pprof-stat strong {
            color: #3b82f6;
        }
        .pprof-stat-sep {
            color: var(--text-secondary);
            font-size: 10px;
        }
        .pprof-help-btn {
            display: flex;
            align-items: center;
            gap: 4px;
            font-size: 11px;
            color: var(--text-secondary);
            background: transparent;
            border: 1px solid var(--border-color);
            padding: 4px 8px;
            border-radius: 4px;
            cursor: pointer;
            transition: all 0.2s;
        }
        .pprof-help-btn:hover {
            background: var(--bg-hover);
            color: var(--text-primary);
        }
        .pprof-help-popup {
            display: none;
            position: absolute;
            right: 0;
            top: 100%;
            margin-top: 4px;
            width: 320px;
            padding: 12px;
            background: #1f2937;
            color: #fff;
            border-radius: 8px;
            font-size: 12px;
            line-height: 1.5;
            box-shadow: 0 4px 12px rgba(0,0,0,0.3);
            z-index: 100;
        }
        .pprof-help-popup.visible {
            display: block;
        }
        .pprof-help-popup code {
            background: rgba(255,255,255,0.1);
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 11px;
        }
        .pprof-compact-header {
            position: relative;
        }
        .pprof-bars {
            display: flex;
            flex-direction: column;
            gap: 4px;
        }
        .pprof-bar-item {
            display: grid;
            grid-template-columns: 150px 1fr;
            gap: 8px;
            align-items: center;
        }
        .pprof-bar-label {
            font-size: 11px;
            color: var(--text-secondary);
            text-overflow: ellipsis;
            overflow: hidden;
            white-space: nowrap;
            font-family: monospace;
        }
        .pprof-bar-container {
            position: relative;
            height: 18px;
            background: var(--bg-secondary);
            border-radius: 3px;
            overflow: hidden;
        }
        .pprof-bar {
            height: 100%;
            border-radius: 3px;
            background: linear-gradient(90deg, #3b82f6, #8b5cf6);
        }
        .pprof-bar-value {
            position: absolute;
            right: 6px;
            top: 50%;
            transform: translateY(-50%);
            font-size: 10px;
            font-weight: 600;
            color: var(--text-primary);
        }
        .pprof-stacks {
            display: flex;
            flex-direction: column;
            gap: 2px;
        }
        .pprof-stack-item {
            background: var(--bg-secondary);
            border-radius: 4px;
            overflow: hidden;
        }
        .pprof-stack-header {
            display: flex;
            align-items: center;
            padding: 6px 10px;
            cursor: pointer;
            gap: 8px;
            transition: background 0.2s;
        }
        .pprof-stack-header:hover {
            background: var(--bg-hover);
        }
        .pprof-stack-count {
            background: #10b981;
            color: white;
            padding: 1px 6px;
            border-radius: 8px;
            font-size: 10px;
            font-weight: 600;
            min-width: 32px;
            text-align: center;
        }
        .pprof-stack-name {
            flex: 1;
            font-size: 11px;
            font-family: monospace;
            color: var(--text-secondary);
            text-overflow: ellipsis;
            overflow: hidden;
            white-space: nowrap;
        }
        .pprof-stack-toggle {
            color: var(--text-secondary);
            font-size: 9px;
            transition: transform 0.2s;
        }
        .pprof-stack-item.expanded .pprof-stack-toggle {
            transform: rotate(90deg);
        }
        .pprof-stack-frames {
            display: none;
            padding: 0 10px 8px 48px;
            font-size: 10px;
            font-family: monospace;
            color: var(--text-secondary);
        }
        .pprof-stack-item.expanded .pprof-stack-frames {
            display: block;
        }
        .pprof-frame {
            padding: 2px 0;
            border-left: 2px solid var(--border-color);
            padding-left: 10px;
            margin-left: 2px;
        }
        .pprof-section {
            margin-bottom: 24px;
        }
        .pprof-section-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 12px;
        }
        .pprof-section-title {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 14px;
            font-weight: 600;
            color: var(--text-primary);
        }
        .pprof-help-link {
            font-size: 11px;
            color: #3b82f6;
            text-decoration: none;
            display: flex;
            align-items: center;
            gap: 4px;
        }
        .pprof-help-link:hover {
            text-decoration: underline;
        }
        .pprof-raw-toggle {
            font-size: 11px;
            color: var(--text-secondary);
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            padding: 4px 10px;
            border-radius: 4px;
            cursor: pointer;
            margin-top: 12px;
        }
        .pprof-raw-toggle:hover {
            background: var(--bg-hover);
        }
        .pprof-raw-content {
            display: none;
            margin-top: 12px;
        }
        .pprof-raw-content.visible {
            display: block;
        }

        /* Comparison */
        .comparison-run-info {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
            margin-top: 20px;
        }
        .run-info-card {
            background: var(--bg-primary);
            padding: 15px;
            border-radius: 8px;
            border: 1px solid var(--border-color);
        }
        .run-info-card h4 {
            margin: 0 0 10px 0;
            color: var(--text-primary);
            font-size: 14px;
        }
        .run-info-details {
            font-size: 12px;
            color: var(--text-secondary);
        }
        .run-info-details > div {
            margin: 5px 0;
        }
        .comparison-grid {
            display: grid;
            grid-template-columns: 1fr 1fr 1fr;
            gap: 20px;
            margin-top: 20px;
        }
        .comparison-col {
            background: var(--bg-primary);
            padding: 20px;
            border-radius: 8px;
            border: 1px solid var(--border-color);
        }
        
        /* Utility */
        .hidden { display: none !important; }
        .theme-toggle {
            background: var(--bg-hover);
            border: 1px solid var(--border-color);
            padding: 8px 12px;
            border-radius: 6px;
            cursor: pointer;
            color: var(--text-primary);
        }

        @media print {
            .theme-toggle, .tabs, .filters, .log-controls, .close-btn { display: none !important; }
            .container { display: block; }
            .tab-content { display: block !important; padding: 0; }
            .card, .chart-wrapper, .table-wrapper { border: 1px solid #ddd; break-inside: avoid; }
            body { background: white; color: black; }
            * { box-shadow: none !important; }
        }
    """

    JS = """
        // ============================================================
        // SECTION: Icons & Utilities
        // ============================================================

        function svgIcon(name, size = 16, color = 'currentColor') {
            const icons = {
                'search': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.35-4.35"></path></svg>`,
                'copy': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`,
                'download': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>`,
                'error': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>`,
                'warning': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>`,
                'info': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="16" x2="12" y2="12"></line><line x1="12" y1="8" x2="12.01" y2="8"></line></svg>`,
                'bug': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m8 2 1.88 1.88"></path><path d="M14.12 3.88 16 2"></path><path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1"></path><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6"></path><path d="M12 20v-9"></path><path d="M6.53 9C4.6 8.8 3 7.1 3 5"></path><path d="M6 13H2"></path><path d="M3 21c0-2.1 1.7-3.9 3.8-4"></path><path d="M20.97 5c0 2.1-1.6 3.8-3.5 4"></path><path d="M22 13h-4"></path><path d="M17.2 17c2.1.1 3.8 1.9 3.8 4"></path></svg>`,
                'chevron-up': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="18 15 12 9 6 15"></polyline></svg>`,
                'chevron-down': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>`,
                'arrow-up': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"></line><polyline points="5 12 12 5 19 12"></polyline></svg>`,
                'arrow-down': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"></line><polyline points="19 12 12 19 5 12"></polyline></svg>`,
                'wrap': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"></polyline><path d="M3 11V9a4 4 0 0 1 4-4h14"></path><polyline points="7 23 3 19 7 15"></polyline><path d="M21 13v2a4 4 0 0 1-4 4H3"></path></svg>`,
                'sun': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>`,
                'moon': `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>`,
            };
            return icons[name] || '';
        }

        // ============================================================
        // SECTION: Global State & Navigation
        // ============================================================

        let currentTab = 'dashboard';

        function switchTab(tabId) {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
            
            document.querySelector(`[onclick="switchTab('${tabId}')"]`).classList.add('active');
            document.getElementById(tabId).classList.add('active');
            currentTab = tabId;
        }

        // Theme
        function toggleTheme() {
            const html = document.documentElement;
            const current = html.getAttribute('data-theme');
            const next = current === 'dark' ? 'light' : 'dark';
            html.setAttribute('data-theme', next);
            localStorage.setItem('theme', next);
            updateThemeButton();
        }

        function updateThemeButton() {
            const theme = document.documentElement.getAttribute('data-theme');
            const btn = document.getElementById('themeToggle');
            if (btn) {
                btn.innerHTML = theme === 'dark' ? svgIcon('sun', 18) : svgIcon('moon', 18);
                btn.title = theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode';
            }
        }
        
        // Init
        document.addEventListener('DOMContentLoaded', () => {
            const savedTheme = localStorage.getItem('theme') ||
                (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
            document.documentElement.setAttribute('data-theme', savedTheme);
            updateThemeButton();

            renderCharts();
        });

        // Filtering
        function filterTable() {
            const search = document.getElementById('searchInput').value.toLowerCase();
            const status = document.getElementById('statusFilter').value;
            const stability = document.getElementById('stabilityFilter').value;
            const label = document.getElementById('labelFilter').value;
            
            const rows = document.querySelectorAll('#resultsTable tbody tr');
            
            rows.forEach(row => {
                const rowData = JSON.parse(row.dataset.json);
                const name = rowData.test_name.toLowerCase();
                
                // Search
                const matchesSearch = name.includes(search);
                
                // Status (check if ANY run matches)
                let matchesStatus = status === 'all';
                if (!matchesStatus) {
                    matchesStatus = Object.values(rowData.runs).some(r => r && r.state === status);
                }
                
                // Stability
                let matchesStability = true;
                if (stability === 'flaky') matchesStability = rowData.is_flaky;
                if (stability === 'stable') matchesStability = !rowData.is_flaky && rowData.fail_count === 0;
                if (stability === 'always-failing') matchesStability = rowData.pass_count === 0;
                
                // Label
                let matchesLabel = label === 'all';
                if (!matchesLabel) {
                    // Check if any run has this label (simplified)
                    matchesLabel = Object.values(rowData.runs).some(r => r && r.labels && r.labels.includes(label));
                }
                
                if (matchesSearch && matchesStatus && matchesStability && matchesLabel) {
                    row.style.display = '';
                } else {
                    row.style.display = 'none';
                }
            });
        }

        // Modal Tabs
        function switchModalTab(tabId) {
            // Update tab buttons
            document.querySelectorAll('.modal-tab').forEach(btn => btn.classList.remove('active'));
            document.querySelector(`[onclick="switchModalTab('${tabId}')"]`).classList.add('active');

            // Update tab content
            document.querySelectorAll('.modal-tab-content').forEach(content => content.classList.remove('active'));
            document.getElementById(tabId).classList.add('active');
        }

        // ============================================================
        // SECTION: Modal & Test Details
        // ============================================================

        function showTestDetails(data) {
            const modal = document.getElementById('testModal');
            const modalBody = document.getElementById('modalContent');

            // Check if this is pivot data (flaky tests) or single test data
            const isPivotData = data.state === undefined;

            // Build tabs - only Summary tab for pivot data
            let html = `
                <div class="modal-tabs">
                    <button class="modal-tab active" onclick="switchModalTab('summary-tab')">Summary</button>
            `;

            if (!isPivotData) {
                html += `
                    <button class="modal-tab" onclick="switchModalTab('artifacts-tab')">Artifacts</button>
                    <button class="modal-tab" onclick="switchModalTab('logs-tab')">Logs</button>
                    <button class="modal-tab" onclick="switchModalTab('resources-tab')">Resources</button>
                `;
            }

            html += `
                </div>
            `;

            // Summary Tab
            html += `
                <div id="summary-tab" class="modal-tab-content active">
                    <div class="test-detail-section">
                        <h4>Test Information</h4>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <div class="detail-label">Test Name</div>
                                <div class="detail-value">${escapeHtml(data.test_name)}</div>
                            </div>
            `;

            // Single test data (from clicking on a result cell)
            if (data.state !== undefined) {
                html += `
                            <div class="detail-item">
                                <div class="detail-label">Status</div>
                                <div class="detail-value"><span class="badge ${data.state}">${data.state}</span></div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Runtime</div>
                                <div class="detail-value">${data.runtime ? data.runtime.toFixed(2) : 'N/A'}s</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Run ID</div>
                                <div class="detail-value"><code>#${data.run_id || 'N/A'}</code></div>
                            </div>
                `;
            } else {
                // Pivot data (from flaky tests - aggregated across runs)
                html += `
                            <div class="detail-item">
                                <div class="detail-label">Total Runs</div>
                                <div class="detail-value">${data.pass_count + data.fail_count}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Pass Count</div>
                                <div class="detail-value"><span class="badge passed">${data.pass_count}</span></div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Fail Count</div>
                                <div class="detail-value"><span class="badge failed">${data.fail_count}</span></div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Flakiness</div>
                                <div class="detail-value">${data.is_flaky ? '<span class="badge skipped">' + (data.flakiness_score ? data.flakiness_score.toFixed(0) + '% Stable' : 'Flaky') + (data.flakiness_pattern ? ' (' + data.flakiness_pattern + ')' : '') + '</span>' : '<span class="badge passed">Stable</span>'}</div>
                            </div>
                `;
            }

            html += `
                        </div>
                    </div>
            `;

            // For pivot data (flaky tests), show run history
            if (data.state === undefined && data.runs) {
                html += `
                    <div class="test-detail-section">
                        <h4>Run History</h4>
                        <div style="overflow-x: auto;">
                            <table style="width: 100%; border-collapse: collapse; margin-top: 10px;">
                                <thead>
                                    <tr style="border-bottom: 2px solid var(--border-color);">
                                        <th style="padding: 8px; text-align: left; font-weight: 600;">Run ID</th>
                                        <th style="padding: 8px; text-align: left; font-weight: 600;">Status</th>
                                        <th style="padding: 8px; text-align: right; font-weight: 600;">Runtime</th>
                                    </tr>
                                </thead>
                                <tbody>
                `;

                Object.entries(data.runs).forEach(([runId, result]) => {
                    html += `
                                    <tr style="border-bottom: 1px solid var(--border-color);">
                                        <td style="padding: 8px;"><code>#${runId}</code></td>
                                        <td style="padding: 8px;"><span class="badge ${result.state}">${result.state}</span></td>
                                        <td style="padding: 8px; text-align: right;">${result.runtime ? result.runtime.toFixed(2) + 's' : 'N/A'}</td>
                                    </tr>
                    `;
                });

                html += `
                                </tbody>
                            </table>
                        </div>
                    </div>
                `;
            }

            if (data.artifact_metadata) {
                const meta = data.artifact_metadata;
                html += `
                    <div class="test-detail-section">
                        <h4>Execution Details</h4>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <div class="detail-label">Namespace</div>
                                <div class="detail-value"><code>${meta.namespace || 'N/A'}</code></div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Start Time</div>
                                <div class="detail-value">${formatTimestamp(meta.start_time)}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">End Time</div>
                                <div class="detail-value">${formatTimestamp(meta.end_time)}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Duration</div>
                                <div class="detail-value">${meta.duration_ms ? (meta.duration_ms / 1e9).toFixed(2) + 's' : 'N/A'}</div>
                            </div>
                        </div>
                    </div>
                `;

                if (meta.artifacts) {
                    html += `
                        <div class="test-detail-section">
                            <h4>Artifact Summary</h4>
                            <div class="detail-grid">
                                <div class="detail-item">
                                    <div class="detail-label">Log Files</div>
                                    <div class="detail-value">${meta.artifacts.log_files?.length || 0}</div>
                                </div>
                                <div class="detail-item">
                                    <div class="detail-label">Resource Files</div>
                                    <div class="detail-value">${meta.artifacts.resource_files?.length || 0}</div>
                                </div>
                                <div class="detail-item">
                                    <div class="detail-label">Event Files</div>
                                    <div class="detail-value">${meta.artifacts.event_files?.length || 0}</div>
                                </div>
                                <div class="detail-item">
                                    <div class="detail-label">Collection Time</div>
                                    <div class="detail-value">${meta.artifacts.collection_time || 'N/A'}</div>
                                </div>
                            </div>
                        </div>
                    `;
                }
            }

            if (data.failure_message) {
                html += `
                    <div class="error-box">
                        <h4>Failure Message</h4>
                        <pre>${escapeHtml(data.failure_message)}</pre>
                    </div>
                `;
            }

            html += `</div>`; // Close summary tab

            // Queue for pprof rendering - declared at function scope for access after DOM update
            let pprofRenderQueue = [];

            // Only show these tabs for single test data (not pivot data)
            if (!isPivotData) {
                // Artifacts Tab
                html += `<div id="artifacts-tab" class="modal-tab-content">`;
            if (data.artifact_metadata?.relative_path) {
                html += `
                    <div class="test-detail-section">
                        <h4>Artifact Location</h4>
                        <div class="detail-value"><code>${data.artifact_metadata.relative_path}</code></div>
                    </div>
                `;
            }
            if (data.artifact_metadata?.artifacts) {
                const artifacts = data.artifact_metadata.artifacts;
                if (artifacts.log_files?.length > 0) {
                    html += `
                        <div class="test-detail-section">
                            <h4>Log Files (${artifacts.log_files.length})</h4>
                            <div class="artifact-list">
                                ${artifacts.log_files.map(f => `<div class="artifact-item"><div class="artifact-name">📄 ${f}</div></div>`).join('')}
                            </div>
                        </div>
                    `;
                }
                if (artifacts.resource_files?.length > 0) {
                    html += `
                        <div class="test-detail-section">
                            <h4>Resource Files (${artifacts.resource_files.length})</h4>
                            <div class="artifact-list">
                                ${artifacts.resource_files.map(f => `<div class="artifact-item"><div class="artifact-name">📦 ${f}</div></div>`).join('')}
                            </div>
                        </div>
                    `;
                }
                if (artifacts.event_files?.length > 0) {
                    html += `
                        <div class="test-detail-section">
                            <h4>Event Files (${artifacts.event_files.length})</h4>
                            <div class="artifact-list">
                                ${artifacts.event_files.map(f => `<div class="artifact-item"><div class="artifact-name">📊 ${f}</div></div>`).join('')}
                            </div>
                        </div>
                    `;
                }
            } else {
                html += `<div class="test-detail-section"><p style="color: var(--text-secondary);">No artifacts available for this test run.</p></div>`;
            }
            html += `</div>`; // Close artifacts tab

            // Logs Tab
            html += `<div id="logs-tab" class="modal-tab-content">`;
            if (data.artifact_metadata?.file_contents) {
                const logFiles = Object.entries(data.artifact_metadata.file_contents)
                    .filter(([name, data]) => data.type === 'log');

                if (logFiles.length > 0) {
                    logFiles.forEach(([filename, fileData], index) => {
                        const viewerId = `testLogViewer${index}`;
                        const navPrevId = `testNavPrevError${index}`;
                        const navNextId = `testNavNextError${index}`;
                        const zoomDisplayId = `testZoomDisplay${index}`;
                        const wrapBtnId = `testWrapToggleBtn${index}`;
                        const searchInputId = `testLogSearchInput${index}`;
                        const searchCounterId = `testLogSearchCounter${index}`;

                        html += `
                            <div class="test-detail-section">
                                <div class="log-controls">
                                    <strong style="color: var(--text-primary);">${filename}</strong>
                                    ${fileData.truncated ? `<span class="artifact-meta">(showing last ${fileData.total_lines > 500 ? 500 : fileData.total_lines} lines of ${fileData.total_lines})</span>` : ''}
                                    <div class="toolbar-separator"></div>
                                    <div class="log-actions-menu">
                                        <button class="log-actions-btn" onclick="toggleActionsMenu('${viewerId}')" title="Actions">⋮</button>
                                        <div id="actionsMenu_${viewerId}" class="log-actions-dropdown">
                                            <div class="log-actions-item" onclick="copyAllLog('${viewerId}')">${svgIcon('copy', 14)} Copy All</div>
                                            <div class="log-actions-item" onclick="copyVisibleLog('${viewerId}')">${svgIcon('copy', 14)} Copy Visible</div>
                                            <div class="log-actions-divider"></div>
                                            <div class="log-actions-item" onclick="downloadLog('${viewerId}', 'txt', '${filename}')">${svgIcon('download', 14)} Download .txt</div>
                                            <div class="log-actions-item" onclick="downloadLog('${viewerId}', 'log', '${filename}')">${svgIcon('download', 14)} Download .log</div>
                                        </div>
                                    </div>
                                    <div class="log-filters-menu">
                                        <button class="log-filters-btn" onclick="toggleFiltersMenu('${viewerId}')" title="Filters">${svgIcon('chevron-down', 14)} Filters</button>
                                        <div id="filtersMenu_${viewerId}" class="log-filters-dropdown">
                                            <div class="log-filters-header">Show:</div>
                                            <label class="log-filter-item">
                                                <input type="checkbox" checked data-level="error" onchange="updateFilterState('${viewerId}')">
                                                <span class="filter-label">${svgIcon('error', 14, '#ef4444')} Errors <span class="filter-count" data-level="error">0</span></span>
                                            </label>
                                            <label class="log-filter-item">
                                                <input type="checkbox" checked data-level="warning" onchange="updateFilterState('${viewerId}')">
                                                <span class="filter-label">${svgIcon('warning', 14, '#f59e0b')} Warnings <span class="filter-count" data-level="warning">0</span></span>
                                            </label>
                                            <label class="log-filter-item">
                                                <input type="checkbox" checked data-level="info" onchange="updateFilterState('${viewerId}')">
                                                <span class="filter-label">${svgIcon('info', 14, '#3b82f6')} Info <span class="filter-count" data-level="info">0</span></span>
                                            </label>
                                            <label class="log-filter-item">
                                                <input type="checkbox" checked data-level="debug" onchange="updateFilterState('${viewerId}')">
                                                <span class="filter-label">${svgIcon('bug', 14, '#22c55e')} Debug <span class="filter-count" data-level="debug">0</span></span>
                                            </label>
                                            <div class="log-filters-actions">
                                                <button class="log-filter-action-btn" onclick="resetFilters('${viewerId}')">Reset</button>
                                                <button class="log-filter-action-btn primary" onclick="applyFilters('${viewerId}')">Apply</button>
                                            </div>
                                        </div>
                                    </div>
                                    <div class="toolbar-separator"></div>
                                    <div class="log-search-controls">
                                        <input type="text" id="${searchInputId}" class="log-search-input" placeholder="Search..." oninput="performSearch('${viewerId}')">
                                        <button class="log-search-btn" onclick="prevSearchResult('${viewerId}')" title="Previous match">${svgIcon('chevron-up', 14)}</button>
                                        <button class="log-search-btn" onclick="nextSearchResult('${viewerId}')" title="Next match">${svgIcon('chevron-down', 14)}</button>
                                        <span id="${searchCounterId}" class="search-counter">0/0</span>
                                    </div>
                                    <div class="log-nav-buttons">
                                        <button class="log-nav-btn" onclick="jumpToTop(document.getElementById('${viewerId}'))" title="Jump to top">
                                            ${svgIcon('arrow-up', 14)} Top
                                        </button>
                                        <button id="${navPrevId}" class="log-nav-btn error-nav" onclick="jumpToPrevError(document.getElementById('${viewerId}'))" title="Jump to previous error">
                                            ${svgIcon('chevron-up', 14)} Prev Error
                                        </button>
                                        <button id="${navNextId}" class="log-nav-btn error-nav" onclick="jumpToNextError(document.getElementById('${viewerId}'))" title="Jump to next error">
                                            ${svgIcon('chevron-down', 14)} Next Error
                                        </button>
                                        <button class="log-nav-btn" onclick="jumpToBottom(document.getElementById('${viewerId}'))" title="Jump to bottom">
                                            ${svgIcon('arrow-down', 14)} Bottom
                                        </button>
                                    </div>
                                </div>
                                <div style="position: relative; margin-top: 10px;">
                                    <div id="${viewerId}" class="log-viewer" style="min-height: 400px; max-height: calc(100vh - 350px); overflow-y: auto;">
                                        ${ansiToHtml(fileData.content)}
                                    </div>
                                    <div class="log-floating-controls">
                                        <div class="floating-control-group">
                                            <div class="floating-zoom-control">
                                                <button class="floating-zoom-btn" onclick="zoom('${viewerId}', 1)" title="Zoom in">+</button>
                                                <span id="${zoomDisplayId}" class="floating-zoom-value">100%</span>
                                                <button class="floating-zoom-btn" onclick="zoom('${viewerId}', -1)" title="Zoom out">−</button>
                                            </div>
                                            <div class="floating-control-divider"></div>
                                            <button id="${wrapBtnId}" class="floating-wrap-btn" onclick="toggleWrap('${viewerId}')" title="Toggle line wrapping">${svgIcon('wrap', 16)}</button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        `;
                    });
                } else {
                    html += `<div class="test-detail-section"><p style="color: var(--text-secondary);">No log files available.</p></div>`;
                }
            } else {
                html += `<div class="test-detail-section"><p style="color: var(--text-secondary);">No logs available for this test run.</p></div>`;
            }
            html += `</div>`; // Close logs tab

            // Resources Tab
            html += `<div id="resources-tab" class="modal-tab-content">`;
            if (data.artifact_metadata?.file_contents) {
                const resourceFiles = Object.entries(data.artifact_metadata.file_contents)
                    .filter(([name, data]) => data.type === 'resource' || data.type === 'events');

                // Separate pprof files from other resources
                const pprofFiles = resourceFiles.filter(([name]) => isPprofFile(name));
                const otherFiles = resourceFiles.filter(([name]) => !isPprofFile(name));

                // Render pprof files with visualization
                if (pprofFiles.length > 0) {
                    let pprofIdx = 0;
                    pprofFiles.forEach(([filename, fileData]) => {
                        const containerId = 'pprof-viz-' + pprofIdx++;
                        const rawId = 'pprof-raw-' + pprofIdx;
                        const isHeap = filename.includes('pprof-heap');
                        const icon = isHeap ? '📊' : '🔄';
                        const title = isHeap ? 'Heap Profile' : 'Goroutine Profile';

                        html += `
                            <div class="pprof-section">
                                <div class="pprof-section-header">
                                    <div class="pprof-section-title">
                                        <span>${icon}</span>
                                        <span>${title}</span>
                                        <span style="font-size: 11px; color: var(--text-secondary); font-weight: 400;">(${filename})</span>
                                    </div>
                                </div>
                                <div id="${containerId}"></div>
                                <button class="pprof-raw-toggle" onclick="document.getElementById('${rawId}').classList.toggle('visible'); this.textContent = this.textContent.includes('Show') ? 'Hide raw output' : 'Show raw output';">
                                    Show raw output
                                </button>
                                <div id="${rawId}" class="pprof-raw-content">
                                    <div class="log-viewer" style="max-height: 300px; overflow: auto;">${escapeHtml(fileData.content)}</div>
                                </div>
                            </div>
                        `;

                        // Queue for rendering after DOM update
                        pprofRenderQueue.push({ filename, content: fileData.content, containerId });
                    });
                }

                // Render other resource files normally
                if (otherFiles.length > 0) {
                    otherFiles.forEach(([filename, fileData]) => {
                        html += `
                            <div class="test-detail-section">
                                <h4>${filename}</h4>
                                <div class="log-viewer">${escapeHtml(fileData.content)}</div>
                            </div>
                        `;
                    });
                }

                if (pprofFiles.length === 0 && otherFiles.length === 0) {
                    html += `<div class="test-detail-section"><p style="color: var(--text-secondary);">No resource files available.</p></div>`;
                }
            } else {
                html += `<div class="test-detail-section"><p style="color: var(--text-secondary);">No resources available for this test run.</p></div>`;
            }
            html += `</div>`; // Close resources tab
            } // End if (!isPivotData)

            modalBody.innerHTML = html;

            // Render pprof visualizations after DOM is updated
            if (pprofRenderQueue && pprofRenderQueue.length > 0) {
                pprofRenderQueue.forEach(item => {
                    renderPprofFile(item.filename, item.content, item.containerId);
                });
            }

            // Apply syntax highlighting to all log viewers
            modalBody.querySelectorAll('.log-viewer').forEach(viewer => {
                viewer.innerHTML = highlightLogSyntax(viewer.innerHTML);
                viewer.classList.add('with-line-numbers');
                // Initialize navigation and display controls for each log viewer
                if (viewer.id) {
                    initLogNavigation(viewer);
                    initDisplayControls(viewer.id);
                }
            });

            // Update modal title
            document.getElementById('testModalTitle').textContent = 'Test Run Details';

            modal.style.display = 'block';
        }

        function formatTimestamp(ts) {
            if (!ts) return 'N/A';
            try {
                return new Date(ts).toLocaleString();
            } catch {
                return ts;
            }
        }

        // Format duration in human-readable format
        function formatDuration(seconds) {
            if (!seconds || seconds < 0) return 'N/A';

            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            const secs = Math.floor(seconds % 60);

            if (hours > 0) {
                return `${hours}h ${minutes}m ${secs}s`;
            } else if (minutes > 0) {
                return `${minutes}m ${secs}s`;
            } else {
                return `${secs}s`;
            }
        }

        // Copy test name to clipboard
        function copyTestName(btn) {
            const testName = btn.getAttribute('data-test-name');

            // Use Clipboard API
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(testName).then(() => {
                    // Visual feedback
                    const originalText = btn.innerHTML;
                    btn.innerHTML = '✓ Copied!';
                    btn.style.background = '#10b981';
                    btn.style.color = 'white';
                    btn.style.borderColor = '#10b981';

                    setTimeout(() => {
                        btn.innerHTML = originalText;
                        btn.style.background = '';
                        btn.style.color = '';
                        btn.style.borderColor = '';
                    }, 1500);
                }).catch(err => {
                    console.error('Failed to copy:', err);
                    btn.innerHTML = '✗ Failed';
                    setTimeout(() => {
                        btn.innerHTML = originalText;
                    }, 1500);
                });
            } else {
                // Fallback for older browsers
                const originalText = btn.innerHTML;
                const textarea = document.createElement('textarea');
                textarea.value = testName;
                textarea.style.position = 'fixed';
                textarea.style.opacity = '0';
                document.body.appendChild(textarea);
                textarea.select();
                try {
                    document.execCommand('copy');
                    btn.innerHTML = '✓ Copied!';
                    btn.style.background = '#10b981';
                    btn.style.color = 'white';
                    setTimeout(() => {
                        btn.innerHTML = originalText;
                        btn.style.background = '';
                        btn.style.color = '';
                    }, 1500);
                } catch (err) {
                    console.error('Fallback copy failed:', err);
                }
                document.body.removeChild(textarea);
            }
        }

        // Copy to clipboard with visual feedback on the clicked element
        function copyWithElementFeedback(text, element) {
            const showSuccess = () => {
                const originalHTML = element.innerHTML;
                element.innerHTML = svgIcon('copy', 12) + ' Copied!';
                element.style.background = '#10b981';
                element.style.color = 'white';
                setTimeout(() => {
                    element.innerHTML = originalHTML;
                    element.style.background = '';
                    element.style.color = '';
                }, 1500);
            };

            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text).then(showSuccess).catch(err => {
                    console.error('Failed to copy:', err);
                    fallbackCopyToClipboard(text, 'Copied!');
                });
            } else {
                fallbackCopyToClipboard(text, 'Copied!');
                showSuccess();
            }
        }

        // Add visual separators between tests
        function addTestSeparators(text) {
            if (!text) return '';

            // Split by Ginkgo test separators (lines with multiple dashes)
            const lines = text.split('\\n');
            const result = [];
            let inTest = false;
            let testName = '';
            let testStatus = '';

            for (let i = 0; i < lines.length; i++) {
                const line = lines[i];

                // Detect test separator line
                if (line.match(/^-{30,}$/)) {
                    // This is a Ginkgo test separator
                    result.push(line);
                    inTest = false;
                    continue;
                }

                // Detect test name (lines that start with test description - typically after separator)
                // Pattern: "Test Description" followed by optional labels like [smoke, slow]
                const testNameMatch = line.match(/^([A-Z][^\\n]+?(?:should|must|can|will|does)[^\\n]*?)(?:\\s+\\[.*?\\])?$/);
                if (testNameMatch && !inTest) {
                    testName = testNameMatch[1].trim();
                    // Add separator and header
                    result.push('<div class="log-test-separator"></div>');
                    result.push(`<div class="log-test-header">${testName}</div>`);
                    result.push(line);
                    inTest = true;
                    continue;
                }

                // Detect test completion with status
                const passedMatch = line.match(/\\[38;5;10m\\[.*?PASSED.*?\\[(\\d+\\.\\d+)\\s*seconds\\].*?\\[0m/);
                const failedMatch = line.match(/\\[38;5;9m\\[.*?FAILED.*?\\[(\\d+\\.\\d+)\\s*seconds\\].*?\\[0m/);

                if (passedMatch || failedMatch) {
                    testStatus = passedMatch ? 'passed' : 'failed';
                    const duration = passedMatch ? passedMatch[1] : (failedMatch ? failedMatch[1] : '');

                    // Update header if we have a test name
                    if (testName && inTest) {
                        const headerIndex = result.lastIndexOf(`<div class="log-test-header">${testName}</div>`);
                        if (headerIndex !== -1) {
                            result[headerIndex] = `<div class="log-test-header">${testName}<span class="test-status ${testStatus}">${testStatus.toUpperCase()} ${duration}s</span></div>`;
                        }
                    }
                    inTest = false;
                    testName = '';
                }

                result.push(line);
            }

            return result.join('\\n');
        }

        // Add line numbers to log output
        function addLineNumbers(text) {
            if (!text) return '';

            const lines = text.split('\\n');
            const numberedLines = lines.map(line => {
                // Skip empty lines at the end
                if (line.trim() === '' && lines[lines.length - 1] === line) {
                    return '';
                }
                return `<div class="log-line"><div class="log-line-number"></div><div class="log-line-content">${line}</div></div>`;
            }).filter(line => line !== '');

            return numberedLines.join('');
        }

        // Highlight log syntax for better readability
        function highlightLogSyntax(text) {
            if (!text) return '';

            // First add test separators
            text = addTestSeparators(text);

            // Then apply syntax highlighting
            text = text
                // PASSED/passed with checkmark
                .replace(/(\\[38;5;10m\\[.*?PASSED.*?\\[0m|\\bPASSED\\b|✅|passed)/gi, '<span class="log-passed">$1</span>')
                // FAILED/failed with cross
                .replace(/(\\[38;5;9m\\[.*?FAILED.*?\\[0m|\\bFAILED\\b|❌|✗|failed)/gi, '<span class="log-failed">$1</span>')
                // STEP markers
                .replace(/(\\[1mSTEP:\\[0m|STEP:)/g, '<span class="log-step">$1</span>')
                // ERROR/error
                .replace(/(\\bERROR\\b|\\berror\\b|\\bError\\b|🔥)/gi, '<span class="log-error">$1</span>')
                // Warnings
                .replace(/(⚠️|WARNING|warning)/gi, '<span class="log-warning">$1</span>')
                // Timestamps like @ 11/19/25 15:35:34.904
                .replace(/(@ \\d{2}\\/\\d{2}\\/\\d{2} \\d{2}:\\d{2}:\\d{2}\\.\\d+)/g, '<span class="log-timestamp">$1</span>')
                // Duration like [35.588 seconds] or 2.5s
                .replace(/(\\[\\d+\\.\\d+ seconds\\]|\\d+\\.\\d+s)/g, '<span class="log-duration">$1</span>')
                // kubectl/running commands
                .replace(/(running:|kubectl|KUBECTL_CMD:)([^\\n]*)/gi, '<span class="log-command">$1$2</span>')
                // Test names (lines starting with test description)
                .replace(/^([▸●] .*$)/gm, '<span class="log-test-name">$1</span>');

            // Finally add line numbers
            return addLineNumbers(text);
        }

        // ============================================================
        // SECTION: Log Viewer State & Navigation
        // ============================================================

        const logViewerStates = new WeakMap();

        function getLogViewerState(viewer) {
            if (!logViewerStates.has(viewer)) {
                logViewerStates.set(viewer, {
                    // Error navigation
                    currentErrorIndex: -1,
                    errorPositions: [],
                    // Display settings
                    currentFontIndex: 1,
                    wrapEnabled: true,
                    // Search state
                    searchIndex: -1,
                    searchMatches: [],
                    originalContent: null,
                    // Filter state
                    filters: { error: true, warning: true, info: true, debug: true }
                });
            }
            return logViewerStates.get(viewer);
        }

        // Legacy accessors for compatibility during refactoring
        function getViewerState(viewer) { return getLogViewerState(viewer); }

        function findErrorsInLog(viewer) {
            const state = getViewerState(viewer);
            state.errorPositions = [];
            const uniqueLines = new Set();
            const errorElements = viewer.querySelectorAll('.log-failed, .log-error');
            errorElements.forEach(elem => {
                const line = elem.closest('.log-line');
                if (line && !uniqueLines.has(line)) {
                    uniqueLines.add(line);
                    state.errorPositions.push(line);
                }
            });
            return state.errorPositions.length;
        }

        function navigateLog(viewer, direction) {
            if (direction === 'top') {
                viewer.scrollTop = 0;
                return;
            }
            if (direction === 'bottom') {
                viewer.scrollTop = viewer.scrollHeight;
                return;
            }

            // Error navigation
            const state = getViewerState(viewer);
            if (state.errorPositions.length === 0) {
                findErrorsInLog(viewer);
            }
            if (state.errorPositions.length === 0) {
                showNotification('No errors found in logs', 'info');
                return;
            }

            if (direction === 'next') {
                state.currentErrorIndex = (state.currentErrorIndex + 1) % state.errorPositions.length;
            } else if (direction === 'prev') {
                state.currentErrorIndex = state.currentErrorIndex <= 0 ? state.errorPositions.length - 1 : state.currentErrorIndex - 1;
            }

            scrollToElement(state.errorPositions[state.currentErrorIndex], viewer);
            updateNavButtons(viewer);
        }

        // Legacy wrappers for compatibility
        function jumpToNextError(viewer) { navigateLog(viewer, 'next'); }
        function jumpToPrevError(viewer) { navigateLog(viewer, 'prev'); }
        function jumpToTop(viewer) { navigateLog(viewer, 'top'); }
        function jumpToBottom(viewer) { navigateLog(viewer, 'bottom'); }

        function scrollToElement(element, container) {
            if (!element || !container) return;

            // Highlight the line temporarily
            const isDark = document.documentElement.classList.contains('theme-dark');
            element.style.transition = 'background 0.3s ease';
            element.style.background = isDark ? 'rgba(59, 130, 246, 0.2)' : 'rgba(147, 197, 253, 0.4)';

            // Scroll to element
            const containerRect = container.getBoundingClientRect();
            const elementRect = element.getBoundingClientRect();
            const scrollTop = container.scrollTop;
            const offset = elementRect.top - containerRect.top + scrollTop - 100;

            container.scrollTo({
                top: offset,
                behavior: 'smooth'
            });

            // Remove highlight after delay
            setTimeout(() => {
                element.style.background = '';
            }, 1500);
        }

        function updateNavButtons(viewer) {
            const state = getViewerState(viewer);
            const errorCount = state.errorPositions.length;

            // Derive button IDs from viewer ID
            const viewerId = viewer.id;
            let prevBtnId, nextBtnId;

            if (viewerId.startsWith('testLogViewer')) {
                // Extract index from testLogViewer0, testLogViewer1, etc.
                const index = viewerId.replace('testLogViewer', '');
                prevBtnId = `testNavPrevError${index}`;
                nextBtnId = `testNavNextError${index}`;
            } else {
                // Default for run log viewer
                prevBtnId = 'navPrevError';
                nextBtnId = 'navNextError';
            }

            const prevBtn = document.getElementById(prevBtnId);
            const nextBtn = document.getElementById(nextBtnId);

            if (prevBtn && nextBtn) {
                const countText = errorCount > 0 ? ` (${state.currentErrorIndex + 1}/${errorCount})` : ' (0)';
                prevBtn.disabled = errorCount === 0;
                nextBtn.disabled = errorCount === 0;

                // Update button text with current position
                if (errorCount > 0) {
                    nextBtn.innerHTML = `<span class="nav-icon">↓</span> Next Error ${countText}`;
                    prevBtn.innerHTML = `<span class="nav-icon">↑</span> Prev Error`;
                }
            }
        }

        function initLogNavigation(viewer) {
            const errorCount = findErrorsInLog(viewer);
            const state = getViewerState(viewer);
            state.currentErrorIndex = -1;
            updateNavButtons(viewer);
            return errorCount;
        }

        // Display control functions
        const fontSizes = ['font-sm', 'font-md', 'font-lg', 'font-xl'];
        const fontLabels = ['85%', '100%', '115%', '130%'];

        function getDisplayState(viewer) { return getLogViewerState(viewer); }

        function zoom(viewerId, delta) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            const state = getDisplayState(viewer);
            const newIndex = state.currentFontIndex + delta;
            if (newIndex < 0 || newIndex >= fontSizes.length) return;

            viewer.classList.remove(fontSizes[state.currentFontIndex]);
            state.currentFontIndex = newIndex;
            viewer.classList.add(fontSizes[state.currentFontIndex]);
            updateZoomDisplay(viewerId);
        }

        function updateZoomDisplay(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            const state = getDisplayState(viewer);

            // Derive zoom display ID from viewer ID
            let displayId;
            if (viewerId.startsWith('testLogViewer')) {
                const index = viewerId.replace('testLogViewer', '');
                displayId = `testZoomDisplay${index}`;
            } else {
                displayId = 'zoomDisplay';
            }

            const display = document.getElementById(displayId);
            if (display) {
                display.textContent = fontLabels[state.currentFontIndex];
            }
        }

        function toggleWrap(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            const state = getDisplayState(viewer);

            // Derive wrap button ID from viewer ID
            let btnId;
            if (viewerId.startsWith('testLogViewer')) {
                const index = viewerId.replace('testLogViewer', '');
                btnId = `testWrapToggleBtn${index}`;
            } else {
                btnId = 'wrapToggleBtn';
            }

            const btn = document.getElementById(btnId);
            if (!btn) return;

            state.wrapEnabled = !state.wrapEnabled;

            if (state.wrapEnabled) {
                viewer.classList.remove('wrap-disabled');
                viewer.classList.add('wrap-enabled');
                btn.classList.add('active');
                btn.title = 'Line wrapping: ON (click to turn off)';
                // Update text for non-floating buttons
                if (!btn.classList.contains('floating-wrap-btn')) {
                    btn.textContent = '↩ Wrap: On';
                }
            } else {
                viewer.classList.remove('wrap-enabled');
                viewer.classList.add('wrap-disabled');
                btn.classList.remove('active');
                btn.title = 'Line wrapping: OFF (click to turn on)';
                // Update text for non-floating buttons
                if (!btn.classList.contains('floating-wrap-btn')) {
                    btn.textContent = '→ Wrap: Off';
                }
            }
        }

        function initDisplayControls(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            // Initialize state
            const state = getDisplayState(viewer);

            // Set initial font size
            viewer.classList.add(fontSizes[state.currentFontIndex]);

            // Set initial wrap state
            if (state.wrapEnabled) {
                viewer.classList.add('wrap-enabled');

                // Set active class for wrap button
                let btnId;
                if (viewerId.startsWith('testLogViewer')) {
                    const index = viewerId.replace('testLogViewer', '');
                    btnId = `testWrapToggleBtn${index}`;
                } else {
                    btnId = 'wrapToggleBtn';
                }
                const btn = document.getElementById(btnId);
                if (btn) {
                    btn.classList.add('active');
                    btn.title = 'Line wrapping: ON (click to turn off)';
                }
            }

            // Add click handlers to line numbers
            viewer.addEventListener('click', (e) => {
                const lineNumber = e.target.closest('.log-line-number');
                if (lineNumber) {
                    const line = lineNumber.closest('.log-line');
                    if (line) {
                        const lineContent = line.querySelector('.log-line-content');
                        if (lineContent) {
                            const lineNum = Array.from(viewer.querySelectorAll('.log-line')).indexOf(line) + 1;
                            copyLineContent(lineNum, lineContent.textContent);
                        }
                    }
                }
            });

            // Make line numbers clickable
            viewer.querySelectorAll('.log-line-number').forEach(num => {
                num.classList.add('log-line-number-clickable');
            });

            // Update display
            updateZoomDisplay(viewerId);
        }

        function copyLineContent(lineNumber, lineContent) {
            const textToCopy = `Line ${lineNumber}: ${lineContent}`;

            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(textToCopy).then(() => {
                    showCopyNotification('Line copied!');
                }).catch(err => {
                    console.error('Failed to copy:', err);
                });
            } else {
                // Fallback
                const textarea = document.createElement('textarea');
                textarea.value = textToCopy;
                textarea.style.position = 'fixed';
                textarea.style.opacity = '0';
                document.body.appendChild(textarea);
                textarea.select();
                try {
                    document.execCommand('copy');
                    showCopyNotification('Line copied!');
                } catch (err) {
                    console.error('Fallback copy failed:', err);
                }
                document.body.removeChild(textarea);
            }
        }

        function showNotification(message, type = 'success') {
            const colors = {
                success: '#10b981',
                warning: '#f59e0b',
                error: '#ef4444',
                info: '#3b82f6'
            };
            const notification = document.createElement('div');
            notification.textContent = message;
            notification.style.cssText = `position:fixed;top:20px;right:20px;background:${colors[type] || colors.info};color:white;padding:10px 18px;border-radius:6px;font-size:13px;z-index:10000;box-shadow:0 4px 12px rgba(0,0,0,0.2);`;
            document.body.appendChild(notification);

            setTimeout(() => {
                notification.style.transition = 'opacity 0.3s ease';
                notification.style.opacity = '0';
                setTimeout(() => document.body.removeChild(notification), 300);
            }, 2000);
        }

        function showCopyNotification(message) { showNotification(message, 'success'); }

        // Search functionality
        function getSearchState(viewer) {
            const state = getLogViewerState(viewer);
            // Map unified state fields to search-specific names for compatibility
            return {
                get currentIndex() { return state.searchIndex; },
                set currentIndex(v) { state.searchIndex = v; },
                get matches() { return state.searchMatches; },
                set matches(v) { state.searchMatches = v; },
                get originalContent() { return state.originalContent; },
                set originalContent(v) { state.originalContent = v; }
            };
        }

        function performSearch(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            const state = getSearchState(viewer);

            // Get search input ID
            let searchInputId, searchCounterId;
            if (viewerId === 'runLogViewer') {
                searchInputId = 'runLogSearchInput';
                searchCounterId = 'runLogSearchCounter';
            } else {
                const index = viewerId.replace('testLogViewer', '');
                searchInputId = `testLogSearchInput${index}`;
                searchCounterId = `testLogSearchCounter${index}`;
            }

            const searchInput = document.getElementById(searchInputId);
            const searchCounter = document.getElementById(searchCounterId);

            if (!searchInput || !searchCounter) return;

            const searchText = searchInput.value.trim();

            // Save original content on first search
            if (!state.originalContent) {
                state.originalContent = viewer.innerHTML;
            }

            // Clear previous search
            viewer.innerHTML = state.originalContent;
            state.matches = [];
            state.currentIndex = -1;

            // Reset error navigation after DOM is rebuilt
            const viewerState = getViewerState(viewer);
            if (viewerState) {
                viewerState.errorPositions = [];
                viewerState.currentErrorIndex = -1;
            }

            if (!searchText) {
                searchCounter.textContent = '0/0';
                return;
            }

            // Find and highlight all matches
            const lines = viewer.querySelectorAll('.log-line');
            let matchCount = 0;

            lines.forEach(line => {
                const contentDiv = line.querySelector('.log-line-content');
                if (!contentDiv) return;

                const text = contentDiv.textContent;
                const lowerText = text.toLowerCase();
                const lowerSearch = searchText.toLowerCase();
                let index = 0;
                let html = '';
                let lastIndex = 0;

                while ((index = lowerText.indexOf(lowerSearch, lastIndex)) !== -1) {
                    // Add text before match
                    html += escapeHtml(text.substring(lastIndex, index));
                    // Add highlighted match
                    html += `<span class="search-highlight" data-match-index="${matchCount}">${escapeHtml(text.substring(index, index + searchText.length))}</span>`;
                    state.matches.push({ line, index: matchCount });
                    matchCount++;
                    lastIndex = index + searchText.length;
                }

                if (lastIndex > 0) {
                    html += escapeHtml(text.substring(lastIndex));
                    contentDiv.innerHTML = html;
                }
            });

            // Update counter
            searchCounter.textContent = matchCount > 0 ? `1/${matchCount}` : '0/0';

            // Highlight first match
            if (matchCount > 0) {
                state.currentIndex = 0;
                highlightCurrentMatch(viewer, 0);
            }
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        function highlightCurrentMatch(viewer, index) {
            // Remove previous current highlight
            const prevCurrent = viewer.querySelector('.search-highlight-current');
            if (prevCurrent) {
                prevCurrent.classList.remove('search-highlight-current');
                prevCurrent.classList.add('search-highlight');
            }

            // Highlight new current match
            const allHighlights = viewer.querySelectorAll('.search-highlight');
            if (index >= 0 && index < allHighlights.length) {
                const currentHighlight = allHighlights[index];
                currentHighlight.classList.remove('search-highlight');
                currentHighlight.classList.add('search-highlight-current');

                // Scroll to match
                const line = currentHighlight.closest('.log-line');
                if (line) {
                    scrollToElement(line, viewer);
                }
            }
        }

        function nextSearchResult(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            const state = getSearchState(viewer);
            if (state.matches.length === 0) return;

            state.currentIndex = (state.currentIndex + 1) % state.matches.length;
            highlightCurrentMatch(viewer, state.currentIndex);
            updateSearchCounter(viewerId, state.currentIndex + 1, state.matches.length);
        }

        function prevSearchResult(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            const state = getSearchState(viewer);
            if (state.matches.length === 0) return;

            state.currentIndex = state.currentIndex - 1;
            if (state.currentIndex < 0) {
                state.currentIndex = state.matches.length - 1;
            }
            highlightCurrentMatch(viewer, state.currentIndex);
            updateSearchCounter(viewerId, state.currentIndex + 1, state.matches.length);
        }

        function updateSearchCounter(viewerId, current, total) {
            let searchCounterId;
            if (viewerId === 'runLogViewer') {
                searchCounterId = 'runLogSearchCounter';
            } else {
                const index = viewerId.replace('testLogViewer', '');
                searchCounterId = `testLogSearchCounter${index}`;
            }

            const searchCounter = document.getElementById(searchCounterId);
            if (searchCounter) {
                searchCounter.textContent = `${current}/${total}`;
            }
        }

        // Actions menu functionality
        function toggleActionsMenu(viewerId) {
            const menuId = `actionsMenu_${viewerId}`;
            const menu = document.getElementById(menuId);
            if (!menu) return;

            // Close all other menus first
            document.querySelectorAll('.log-actions-dropdown').forEach(m => {
                if (m.id !== menuId) {
                    m.classList.remove('show');
                }
            });

            // Toggle current menu
            menu.classList.toggle('show');
        }

        // Close menus when clicking outside
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.log-actions-menu')) {
                document.querySelectorAll('.log-actions-dropdown').forEach(m => {
                    m.classList.remove('show');
                });
            }
            if (!e.target.closest('.log-filters-menu')) {
                document.querySelectorAll('.log-filters-dropdown').forEach(m => {
                    m.classList.remove('show');
                });
            }
        });

        function copyAllLog(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            // Get all text content from log lines
            const lines = viewer.querySelectorAll('.log-line-content');
            const text = Array.from(lines).map(line => line.textContent).join('\\n');

            copyToClipboard(text, 'All log content copied!');
            toggleActionsMenu(viewerId); // Close menu after action
        }

        function copyVisibleLog(viewerId) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            // Get visible text content (respecting any filters/search)
            const text = viewer.textContent;

            copyToClipboard(text, 'Visible log content copied!');
            toggleActionsMenu(viewerId);
        }

        function copyToClipboard(text, message) {
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text).then(() => {
                    showCopyNotification(message);
                }).catch(err => {
                    console.error('Failed to copy:', err);
                    fallbackCopyToClipboard(text, message);
                });
            } else {
                fallbackCopyToClipboard(text, message);
            }
        }

        function fallbackCopyToClipboard(text, message) {
            const textarea = document.createElement('textarea');
            textarea.value = text;
            textarea.style.position = 'fixed';
            textarea.style.opacity = '0';
            document.body.appendChild(textarea);
            textarea.select();
            try {
                document.execCommand('copy');
                showCopyNotification(message);
            } catch (err) {
                console.error('Fallback copy failed:', err);
            }
            document.body.removeChild(textarea);
        }

        function downloadLog(viewerId, extension, filename) {
            const viewer = document.getElementById(viewerId);
            if (!viewer) return;

            // Get all text content from log lines
            const lines = viewer.querySelectorAll('.log-line-content');
            const text = Array.from(lines).map(line => line.textContent).join('\\n');

            // Create blob and download
            const blob = new Blob([text], { type: 'text/plain' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `${filename}.${extension}`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

            showCopyNotification(`Downloaded ${filename}.${extension}`);
            toggleActionsMenu(viewerId);
        }

        // Filters functionality
        function getFilterState(viewer) {
            return getLogViewerState(viewer).filters;
        }

        function toggleFiltersMenu(viewerId) {
            const menuId = `filtersMenu_${viewerId}`;
            const menu = document.getElementById(menuId);
            if (!menu) return;

            // Close all other menus
            document.querySelectorAll('.log-filters-dropdown, .log-actions-dropdown').forEach(m => {
                if (m.id !== menuId) {
                    m.classList.remove('show');
                }
            });

            // Toggle current menu
            const wasOpen = menu.classList.contains('show');
            menu.classList.toggle('show');

            // Count logs when opening
            if (!wasOpen) {
                const viewer = document.getElementById(viewerId);
                if (viewer) {
                    countLogLevels(viewer, menuId);
                }
            }
        }

        function countLogLevels(viewer, menuId) {
            const menu = document.getElementById(menuId);
            if (!menu) return;

            const counts = {
                error: 0,
                warning: 0,
                info: 0,
                debug: 0
            };

            const lines = viewer.querySelectorAll('.log-line');
            lines.forEach(line => {
                const text = line.textContent.toLowerCase();
                if (text.includes('error') || text.includes('failed') || text.includes('✗') || text.includes('❌')) {
                    counts.error++;
                } else if (text.includes('warn')) {
                    counts.warning++;
                } else if (text.includes('info') || text.includes('✓') || text.includes('✅')) {
                    counts.info++;
                } else if (text.includes('debug')) {
                    counts.debug++;
                } else {
                    counts.info++; // Default to info
                }
            });

            // Update counts in UI
            Object.keys(counts).forEach(level => {
                const countSpan = menu.querySelector(`.filter-count[data-level="${level}"]`);
                if (countSpan) {
                    countSpan.textContent = counts[level];
                }
            });
        }

        function updateFilterState(viewerId) {
            // This is called on checkbox change, but we apply filters on "Apply" click
            // Just keep it for future real-time filtering if needed
        }

        function applyFilters(viewerId) {
            const viewer = document.getElementById(viewerId);
            const menuId = `filtersMenu_${viewerId}`;
            const menu = document.getElementById(menuId);
            if (!viewer || !menu) return;

            // Get checked states
            const checkboxes = menu.querySelectorAll('input[type="checkbox"]');
            const enabledLevels = {
                error: false,
                warning: false,
                info: false,
                debug: false
            };

            checkboxes.forEach(cb => {
                const level = cb.getAttribute('data-level');
                if (level) {
                    enabledLevels[level] = cb.checked;
                }
            });

            // Save state
            const state = getFilterState(viewer);
            Object.assign(state, enabledLevels);

            // Apply filters to log lines
            const lines = viewer.querySelectorAll('.log-line');
            lines.forEach(line => {
                const text = line.textContent.toLowerCase();
                let shouldShow = false;

                if (enabledLevels.error && (text.includes('error') || text.includes('failed') || text.includes('✗') || text.includes('❌'))) {
                    shouldShow = true;
                } else if (enabledLevels.warning && text.includes('warn')) {
                    shouldShow = true;
                } else if (enabledLevels.debug && text.includes('debug')) {
                    shouldShow = true;
                } else if (enabledLevels.info) {
                    // Show everything else if info is enabled
                    shouldShow = true;
                }

                line.style.display = shouldShow ? '' : 'none';
            });

            // Close menu
            toggleFiltersMenu(viewerId);
        }

        function resetFilters(viewerId) {
            const viewer = document.getElementById(viewerId);
            const menuId = `filtersMenu_${viewerId}`;
            const menu = document.getElementById(menuId);
            if (!viewer || !menu) return;

            // Check all checkboxes
            const checkboxes = menu.querySelectorAll('input[type="checkbox"]');
            checkboxes.forEach(cb => {
                cb.checked = true;
            });

            // Show all lines
            const lines = viewer.querySelectorAll('.log-line');
            lines.forEach(line => {
                line.style.display = '';
            });

            // Reset state
            const state = getFilterState(viewer);
            state.error = true;
            state.warning = true;
            state.info = true;
            state.debug = true;
        }

        function showRunDetails(runData) {
            const modal = document.getElementById('testModal');
            const modalBody = document.getElementById('modalContent');

            const passRate = runData.total_tests > 0 ? ((runData.passed_tests / runData.total_tests) * 100).toFixed(1) : '0.0';

            let html = `
                <div class="modal-tabs">
                    <button class="modal-tab active" onclick="switchModalTab('run-summary-tab')">Summary</button>
                    <button class="modal-tab" onclick="switchModalTab('run-logs-tab')">Full Run Log</button>
                </div>

                <div id="run-summary-tab" class="modal-tab-content active">
                    <div class="test-detail-section">
                        <h4>Run Information</h4>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <div class="detail-label">Run ID</div>
                                <div class="detail-value"><code>#${runData.run_id}</code></div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Start Time</div>
                                <div class="detail-value">${formatTimestamp(runData.start_time)}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Total Runtime</div>
                                <div class="detail-value">${formatDuration(runData.total_runtime)}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Pass Rate</div>
                                <div class="detail-value">${passRate}%</div>
                            </div>
                        </div>
                    </div>

                    <div class="test-detail-section">
                        <h4>Test Results</h4>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <div class="detail-label">Total Tests</div>
                                <div class="detail-value">${runData.total_tests}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Passed</div>
                                <div class="detail-value" style="color: #10b981;">${runData.passed_tests}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Failed</div>
                                <div class="detail-value" style="color: #ef4444;">${runData.failed_tests}</div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Skipped</div>
                                <div class="detail-value">${runData.total_tests - runData.passed_tests - runData.failed_tests}</div>
                            </div>
                        </div>
                    </div>

                    <div class="test-detail-section">
                        <h4>Git Information</h4>
                        <div class="detail-grid">
                            <div class="detail-item">
                                <div class="detail-label">Branch</div>
                                <div class="detail-value"><code>${runData.git_branch || 'unknown'}</code></div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Commit</div>
                                <div class="detail-value"><code>${runData.git_commit || 'unknown'}</code></div>
                            </div>
                            <div class="detail-item">
                                <div class="detail-label">Working Directory</div>
                                <div class="detail-value">${runData.git_dirty ? '<span style="color: #ef4444;">● Dirty</span>' : '<span style="color: #10b981;">✓ Clean</span>'}</div>
                            </div>
                        </div>
                    </div>
                </div>

                <div id="run-logs-tab" class="modal-tab-content">
                    <div class="test-detail-section">
                        <div class="log-controls">
                            <strong style="color: var(--text-primary);">Complete Test Run Output</strong>
                            <div class="toolbar-separator"></div>
                            <div class="log-actions-menu">
                                <button class="log-actions-btn" onclick="toggleActionsMenu('runLogViewer')" title="Actions">⋮</button>
                                <div id="actionsMenu_runLogViewer" class="log-actions-dropdown">
                                    <div class="log-actions-item" onclick="copyAllLog('runLogViewer')">${svgIcon('copy', 14)} Copy All</div>
                                    <div class="log-actions-item" onclick="copyVisibleLog('runLogViewer')">${svgIcon('copy', 14)} Copy Visible</div>
                                    <div class="log-actions-divider"></div>
                                    <div class="log-actions-item" onclick="downloadLog('runLogViewer', 'txt', 'test-run-output')">${svgIcon('download', 14)} Download .txt</div>
                                    <div class="log-actions-item" onclick="downloadLog('runLogViewer', 'log', 'test-run-output')">${svgIcon('download', 14)} Download .log</div>
                                </div>
                            </div>
                            <div class="log-filters-menu">
                                <button class="log-filters-btn" onclick="toggleFiltersMenu('runLogViewer')" title="Filters">${svgIcon('chevron-down', 14)} Filters</button>
                                <div id="filtersMenu_runLogViewer" class="log-filters-dropdown">
                                    <div class="log-filters-header">Show:</div>
                                    <label class="log-filter-item">
                                        <input type="checkbox" checked data-level="error" onchange="updateFilterState('runLogViewer')">
                                        <span class="filter-label">${svgIcon('error', 14, '#ef4444')} Errors <span class="filter-count" data-level="error">0</span></span>
                                    </label>
                                    <label class="log-filter-item">
                                        <input type="checkbox" checked data-level="warning" onchange="updateFilterState('runLogViewer')">
                                        <span class="filter-label">${svgIcon('warning', 14, '#f59e0b')} Warnings <span class="filter-count" data-level="warning">0</span></span>
                                    </label>
                                    <label class="log-filter-item">
                                        <input type="checkbox" checked data-level="info" onchange="updateFilterState('runLogViewer')">
                                        <span class="filter-label">${svgIcon('info', 14, '#3b82f6')} Info <span class="filter-count" data-level="info">0</span></span>
                                    </label>
                                    <label class="log-filter-item">
                                        <input type="checkbox" checked data-level="debug" onchange="updateFilterState('runLogViewer')">
                                        <span class="filter-label">${svgIcon('bug', 14, '#22c55e')} Debug <span class="filter-count" data-level="debug">0</span></span>
                                    </label>
                                    <div class="log-filters-actions">
                                        <button class="log-filter-action-btn" onclick="resetFilters('runLogViewer')">Reset</button>
                                        <button class="log-filter-action-btn primary" onclick="applyFilters('runLogViewer')">Apply</button>
                                    </div>
                                </div>
                            </div>
                            <div class="toolbar-separator"></div>
                            <div class="log-search-controls">
                                <input type="text" id="runLogSearchInput" class="log-search-input" placeholder="Search..." oninput="performSearch('runLogViewer')">
                                <button class="log-search-btn" onclick="prevSearchResult('runLogViewer')" title="Previous match">${svgIcon('chevron-up', 14)}</button>
                                <button class="log-search-btn" onclick="nextSearchResult('runLogViewer')" title="Next match">${svgIcon('chevron-down', 14)}</button>
                                <span id="runLogSearchCounter" class="search-counter">0/0</span>
                            </div>
                            <div class="log-nav-buttons">
                                <button class="log-nav-btn" onclick="jumpToTop(document.getElementById('runLogViewer'))" title="Jump to top">
                                    ${svgIcon('arrow-up', 14)} Top
                                </button>
                                <button id="navPrevError" class="log-nav-btn error-nav" onclick="jumpToPrevError(document.getElementById('runLogViewer'))" title="Jump to previous error">
                                    ${svgIcon('chevron-up', 14)} Prev Error
                                </button>
                                <button id="navNextError" class="log-nav-btn error-nav" onclick="jumpToNextError(document.getElementById('runLogViewer'))" title="Jump to next error">
                                    ${svgIcon('chevron-down', 14)} Next Error
                                </button>
                                <button class="log-nav-btn" onclick="jumpToBottom(document.getElementById('runLogViewer'))" title="Jump to bottom">
                                    ${svgIcon('arrow-down', 14)} Bottom
                                </button>
                            </div>
                        </div>
                        <div style="position: relative; margin-top: 10px;">
                            <div id="runLogViewer" class="log-viewer" style="min-height: 400px; max-height: calc(100vh - 350px); overflow-y: auto;">
                                ${ansiToHtml(escapeHtml(runData.test_output_log || 'No logs available'))}
                            </div>
                            <div class="log-floating-controls">
                                <div class="floating-control-group">
                                    <div class="floating-zoom-control">
                                        <button class="floating-zoom-btn" onclick="zoom('runLogViewer', 1)" title="Zoom in">+</button>
                                        <span id="zoomDisplay" class="floating-zoom-value">100%</span>
                                        <button class="floating-zoom-btn" onclick="zoom('runLogViewer', -1)" title="Zoom out">−</button>
                                    </div>
                                    <div class="floating-control-divider"></div>
                                    <button id="wrapToggleBtn" class="floating-wrap-btn" onclick="toggleWrap('runLogViewer')" title="Toggle line wrapping">${svgIcon('wrap', 16)}</button>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            `;

            modalBody.innerHTML = html;

            // Apply syntax highlighting to run log viewer
            const runLogViewer = document.getElementById('runLogViewer');
            if (runLogViewer) {
                runLogViewer.innerHTML = highlightLogSyntax(runLogViewer.innerHTML);
                runLogViewer.classList.add('with-line-numbers');
                // Initialize navigation and display controls after syntax highlighting
                initLogNavigation(runLogViewer);
                initDisplayControls('runLogViewer');
            }

            document.getElementById('testModalTitle').textContent = 'Run #' + runData.run_id + ' Details';
            modal.style.display = 'block';
        }

        function closeModal(id) {
            if (!id) id = 'testModal';
            document.getElementById(id).style.display = 'none';
        }

        // ============================================================
        // SECTION: pprof Visualization
        // ============================================================

        function parsePprofHeap(content) {
            const lines = content.split('\\n');
            const result = { totalSize: 0, totalObjects: 0, entries: [] };

            // Parse header: heap profile: 12: 323920 [125: 1251160] @ heap/1048576
            const headerMatch = lines[0]?.match(/heap profile: (\\d+): (\\d+) \\[(\\d+): (\\d+)\\]/);
            if (headerMatch) {
                result.liveObjects = parseInt(headerMatch[1]);
                result.liveSize = parseInt(headerMatch[2]);
                result.totalObjects = parseInt(headerMatch[3]);
                result.totalSize = parseInt(headerMatch[4]);
            }

            // Parse entries
            let currentEntry = null;
            for (let i = 1; i < lines.length; i++) {
                const line = lines[i];
                // Entry line: 1: 278528 [1: 278528] @ 0x...
                const entryMatch = line.match(/^(\\d+): (\\d+) \\[(\\d+): (\\d+)\\] @/);
                if (entryMatch) {
                    if (currentEntry) result.entries.push(currentEntry);
                    currentEntry = {
                        liveObjects: parseInt(entryMatch[1]),
                        liveSize: parseInt(entryMatch[2]),
                        totalObjects: parseInt(entryMatch[3]),
                        totalSize: parseInt(entryMatch[4]),
                        stack: []
                    };
                } else if (line.startsWith('#') && currentEntry) {
                    // Stack frame: #	0x6dfa4b	k8s.io/api/core/v1.(*Secret).Unmarshal+0x100b
                    const frameMatch = line.match(/#\\s+0x[0-9a-f]+\\s+(.+?)(?:\\s+\\/|$)/);
                    if (frameMatch) {
                        currentEntry.stack.push(frameMatch[1].trim());
                    }
                }
            }
            if (currentEntry) result.entries.push(currentEntry);

            // Sort by live size descending
            result.entries.sort((a, b) => b.liveSize - a.liveSize);
            return result;
        }

        function parsePprofGoroutine(content) {
            const lines = content.split('\\n');
            const result = { totalGoroutines: 0, stacks: [] };

            // Parse header: goroutine profile: total 15
            const headerMatch = lines[0]?.match(/goroutine profile: total (\\d+)/);
            if (headerMatch) {
                result.totalGoroutines = parseInt(headerMatch[1]);
            }

            // Parse entries
            let currentStack = null;
            for (let i = 1; i < lines.length; i++) {
                const line = lines[i];
                // Entry line: 1 @ 0x... or count @ 0x...
                const entryMatch = line.match(/^(\\d+) @/);
                if (entryMatch) {
                    if (currentStack) result.stacks.push(currentStack);
                    currentStack = {
                        count: parseInt(entryMatch[1]),
                        frames: []
                    };
                } else if (line.startsWith('#') && currentStack) {
                    const frameMatch = line.match(/#\\s+0x[0-9a-f]+\\s+(.+?)(?:\\s+\\/|$)/);
                    if (frameMatch) {
                        currentStack.frames.push(frameMatch[1].trim());
                    }
                }
            }
            if (currentStack) result.stacks.push(currentStack);

            // Sort by count descending
            result.stacks.sort((a, b) => b.count - a.count);
            return result;
        }

        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        }

        function renderPprofHeap(parsed, containerId) {
            const container = document.getElementById(containerId);
            if (!container || !parsed) return;

            const topEntries = parsed.entries.slice(0, 8);
            const maxSize = topEntries[0]?.liveSize || 1;

            let html = `
                <div class="pprof-compact-header">
                    <div class="pprof-stats">
                        <span class="pprof-stat"><strong>${formatBytes(parsed.liveSize)}</strong> live</span>
                        <span class="pprof-stat-sep">•</span>
                        <span class="pprof-stat"><strong>${parsed.liveObjects?.toLocaleString() || 0}</strong> objects</span>
                        <span class="pprof-stat-sep">•</span>
                        <span class="pprof-stat"><strong>${formatBytes(parsed.totalSize)}</strong> total allocated</span>
                    </div>
                    <button class="pprof-help-btn" onclick="this.nextElementSibling.classList.toggle('visible')">
                        ${svgIcon('info', 14)} Help
                    </button>
                    <div class="pprof-help-popup">
                        <strong>How to interpret heap profile:</strong><br>
                        • <strong>Live memory</strong> — currently allocated and in use<br>
                        • <strong>Top allocators</strong> — functions allocating most memory<br>
                        • Look for unexpected large allocations or memory leaks<br><br>
                        <strong>Deep analysis:</strong><br>
                        <code>go tool pprof heap.pb.gz</code><br>
                        Commands: <code>top</code>, <code>web</code>, <code>list funcName</code>
                    </div>
                </div>
                <div class="pprof-bars">
            `;

            topEntries.forEach((entry, idx) => {
                const pct = (entry.liveSize / maxSize * 100).toFixed(0);
                const funcName = entry.stack[0] || 'unknown';
                const shortName = funcName.split('.').pop() || funcName;
                html += `
                    <div class="pprof-bar-item">
                        <div class="pprof-bar-label" title="${escapeHtml(funcName)}">${escapeHtml(shortName)}</div>
                        <div class="pprof-bar-container">
                            <div class="pprof-bar" style="width: ${pct}%;"></div>
                            <span class="pprof-bar-value">${formatBytes(entry.liveSize)}</span>
                        </div>
                    </div>
                `;
            });

            html += '</div>';
            container.innerHTML = html;
        }

        function renderPprofGoroutine(parsed, containerId) {
            const container = document.getElementById(containerId);
            if (!container || !parsed) return;

            let html = `
                <div class="pprof-compact-header">
                    <div class="pprof-stats">
                        <span class="pprof-stat"><strong>${parsed.totalGoroutines}</strong> goroutines</span>
                        <span class="pprof-stat-sep">•</span>
                        <span class="pprof-stat"><strong>${parsed.stacks.length}</strong> unique stacks</span>
                    </div>
                    <button class="pprof-help-btn" onclick="this.nextElementSibling.classList.toggle('visible')">
                        ${svgIcon('info', 14)} Help
                    </button>
                    <div class="pprof-help-popup">
                        <strong>How to interpret goroutine profile:</strong><br>
                        • <strong>Count (Nx)</strong> — number of goroutines with same stack<br>
                        • High counts may indicate goroutine leaks<br>
                        • Click stack to see full call trace<br><br>
                        <strong>What to look for:</strong><br>
                        • Blocked goroutines (waiting on channels/mutexes)<br>
                        • Unexpected goroutine accumulation over time<br>
                        • Goroutines stuck in infinite loops
                    </div>
                </div>
                <div class="pprof-stacks">
            `;

            parsed.stacks.slice(0, 12).forEach((stack, idx) => {
                const topFrame = stack.frames[0] || 'unknown';
                const shortName = topFrame.split('.').pop() || topFrame;
                html += `
                    <div class="pprof-stack-item">
                        <div class="pprof-stack-header" onclick="this.parentElement.classList.toggle('expanded')">
                            <span class="pprof-stack-count">${stack.count}x</span>
                            <span class="pprof-stack-name" title="${escapeHtml(topFrame)}">${escapeHtml(shortName)}</span>
                            <span class="pprof-stack-toggle">▶</span>
                        </div>
                        <div class="pprof-stack-frames">
                            ${stack.frames.map(f => `<div class="pprof-frame">${escapeHtml(f)}</div>`).join('')}
                        </div>
                    </div>
                `;
            });

            html += '</div>';
            container.innerHTML = html;
        }

        function isPprofFile(filename) {
            return filename.includes('pprof-heap') || filename.includes('pprof-goroutine');
        }

        function renderPprofFile(filename, content, containerId) {
            if (filename.includes('pprof-heap')) {
                renderPprofHeap(parsePprofHeap(content), containerId);
            } else if (filename.includes('pprof-goroutine')) {
                renderPprofGoroutine(parsePprofGoroutine(content), containerId);
            }
        }

        function copyAllTestNames() {
            const rows = document.querySelectorAll('#resultsTable tbody tr');
            let names = [];
            rows.forEach(row => {
                if (row.style.display !== 'none') {
                    const data = JSON.parse(row.dataset.json);
                    names.push(data.test_name);
                }
            });

            if (names.length > 0) {
                navigator.clipboard.writeText(names.join('\\n')).then(() => {
                    const btn = document.querySelector('button[onclick="copyAllTestNames()"]');
                    const originalText = btn.innerHTML;
                    btn.innerHTML = '✓ Copied ' + names.length + ' tests!';
                    setTimeout(() => {
                        btn.innerHTML = originalText;
                    }, 2000);
                });
            } else {
                showNotification('No tests visible to copy', 'warning');
            }
        }
        
        function ansiToHtml(text) {
            if (!text) return '';
            
            // Basic colors
            const colors = {
                30: 'black', 31: '#ef4444', 32: '#10b981', 33: '#f59e0b',
                34: '#3b82f6', 35: '#d946ef', 36: '#06b6d4', 37: '#f8fafc',
                90: '#64748b', 91: '#f87171', 92: '#34d399', 93: '#fbbf24',
                94: '#60a5fa', 95: '#e879f9', 96: '#22d3ee', 97: '#ffffff'
            };
            
            // Backgrounds (40-47: normal, 100-107: bright)
            const bgColors = {
                40: '#000000', 41: '#7f1d1d', 42: '#064e3b', 43: '#78350f',
                44: '#1e3a8a', 45: '#581c87', 46: '#164e63', 47: '#e5e7eb',
                100: '#374151', 101: '#991b1b', 102: '#065f46', 103: '#92400e',
                104: '#1e40af', 105: '#6b21a8', 106: '#0e7490', 107: '#f9fafb'
            };

            let html = '';
            let currentStyle = [];
            
            // Split by escape sequences
            const parts = text.split(/(\\x1b\\[[0-9;]*m)/g);
            
            for (const part of parts) {
                if (part.startsWith('\\x1b[')) {
                    const codes = part.slice(2, -1).split(';').map(Number);
                    for (const code of codes) {
                        if (code === 0) currentStyle = [];
                        else if (code === 1) currentStyle.push('font-weight:bold');
                        else if (code === 2) currentStyle.push('opacity:0.7');
                        else if (code === 4) currentStyle.push('text-decoration:underline');
                        else if (colors[code]) currentStyle.push(`color:${colors[code]}`);
                        else if (bgColors[code]) currentStyle.push(`background-color:${bgColors[code]}`);
                    }
                } else {
                    if (part) {
                        const style = currentStyle.length ? ` style="${currentStyle.join(';')}"` : '';
                        html += `<span${style}>${escapeHtml(part)}</span>`;
                    }
                }
            }
            return html || text;
        }

        // ============================================================
        // SECTION: Charts & Comparison
        // ============================================================

        function renderCharts() {
            if (typeof Chart === 'undefined') return;
            
            const ctxRate = document.getElementById('passRateChart').getContext('2d');
            const ctxDuration = document.getElementById('durationChart').getContext('2d');
            
            new Chart(ctxRate, {
                type: 'line',
                data: window.chartData.passRate,
                options: { responsive: true, maintainAspectRatio: false }
            });
            
            new Chart(ctxDuration, {
                type: 'line',
                data: window.chartData.duration,
                options: { responsive: true, maintainAspectRatio: false }
            });
        }
        
        // Comparison
        // Get run data by ID
        function getRunData(runId) {
            return window.chartData.runs.find(r => r.run_id == runId);
        }

        // Update run info cards
        function updateRunInfo() {
            const runA = document.getElementById('runASelect').value;
            const runB = document.getElementById('runBSelect').value;

            const runAData = getRunData(runA);
            const runBData = getRunData(runB);

            if (runAData) {
                const passRate = ((runAData.passed / runAData.total) * 100).toFixed(1);
                document.getElementById('runAInfo').innerHTML = `
                    <div><strong>Run ID:</strong> ${runAData.run_id}</div>
                    <div><strong>Date:</strong> ${new Date(runAData.timestamp).toLocaleString()}</div>
                    <div><strong>Total Tests:</strong> ${runAData.total}</div>
                    <div><strong>Passed:</strong> <span style="color: #10b981;">${runAData.passed}</span></div>
                    <div><strong>Failed:</strong> <span style="color: #ef4444;">${runAData.failed}</span></div>
                    <div><strong>Pass Rate:</strong> ${passRate}%</div>
                `;
            }

            if (runBData) {
                const passRate = ((runBData.passed / runBData.total) * 100).toFixed(1);
                document.getElementById('runBInfo').innerHTML = `
                    <div><strong>Run ID:</strong> ${runBData.run_id}</div>
                    <div><strong>Date:</strong> ${new Date(runBData.timestamp).toLocaleString()}</div>
                    <div><strong>Total Tests:</strong> ${runBData.total}</div>
                    <div><strong>Passed:</strong> <span style="color: #10b981;">${runBData.passed}</span></div>
                    <div><strong>Failed:</strong> <span style="color: #ef4444;">${runBData.failed}</span></div>
                    <div><strong>Pass Rate:</strong> ${passRate}%</div>
                `;
            }
        }

        function compareRuns() {
            const runA = document.getElementById('runASelect').value;
            const runB = document.getElementById('runBSelect').value;

            if (runA === runB) {
                showNotification('Please select different runs to compare', 'warning');
                return;
            }

            const newFailures = [];
            const fixedTests = [];
            const regressions = [];

            let totalRuntimeA = 0;
            let totalRuntimeB = 0;
            let testsCompared = 0;

            window.pivotData.forEach(row => {
                const resA = row.runs[runA];
                const resB = row.runs[runB];

                if (resA && resB) {
                    // Track runtime
                    if (resA.runtime) totalRuntimeA += resA.runtime;
                    if (resB.runtime) totalRuntimeB += resB.runtime;
                    testsCompared++;

                    // New failures: A passed, B failed
                    if (resA.state === 'passed' && resB.state === 'failed') {
                        newFailures.push({...row, runtimeA: resA.runtime, runtimeB: resB.runtime});
                    }
                    // Fixed tests: A failed, B passed
                    if (resA.state === 'failed' && resB.state === 'passed') {
                        fixedTests.push({...row, runtimeA: resA.runtime, runtimeB: resB.runtime});
                    }
                    // Regressions: was flaky or occasionally failing, now consistently failing
                    if (row.is_flaky && resB.state === 'failed' && row.fail_count > 1) {
                        regressions.push({...row, runtimeA: resA.runtime, runtimeB: resB.runtime});
                    }
                }
            });

            // Update summary cards
            document.getElementById('comparisonSummary').style.display = 'grid';
            document.getElementById('newFailuresCount').textContent = newFailures.length;
            document.getElementById('fixedTestsCount').textContent = fixedTests.length;
            document.getElementById('regressionsCount').textContent = regressions.length;

            // Calculate runtime diff
            const runtimeDiff = totalRuntimeB - totalRuntimeA;
            const runtimeDiffPercent = totalRuntimeA > 0 ? ((runtimeDiff / totalRuntimeA) * 100).toFixed(1) : 0;
            const runtimeColor = runtimeDiff > 0 ? '#ef4444' : runtimeDiff < 0 ? '#10b981' : 'var(--text-primary)';
            const runtimeSign = runtimeDiff > 0 ? '+' : '';
            document.getElementById('runtimeDiff').innerHTML = `<span style="color: ${runtimeColor}">${runtimeSign}${runtimeDiff.toFixed(1)}s (${runtimeSign}${runtimeDiffPercent}%)</span>`;

            // Render lists
            document.getElementById('newFailuresList').innerHTML = newFailures.map(r =>
                `<div style="padding: 8px; border-bottom: 1px solid var(--border-color); cursor: pointer;" onclick='showTestDetails(${JSON.stringify({test_name: r.test_name, state: "failed", runtime: r.runtimeB, run_id: runB})})'>
                    <div style="font-size: 13px;">${escapeHtml(r.test_name)}</div>
                    <div style="font-size: 11px; color: var(--text-secondary); margin-top: 4px;">
                        Runtime: ${r.runtimeA ? r.runtimeA.toFixed(2) + 's' : 'N/A'} → ${r.runtimeB ? r.runtimeB.toFixed(2) + 's' : 'N/A'}
                    </div>
                </div>`
            ).join('') || '<div style="color: var(--text-secondary); padding: 20px; text-align: center;">No new failures</div>';

            document.getElementById('fixedTestsList').innerHTML = fixedTests.map(r =>
                `<div style="padding: 8px; border-bottom: 1px solid var(--border-color); cursor: pointer;" onclick='showTestDetails(${JSON.stringify({test_name: r.test_name, state: "passed", runtime: r.runtimeB, run_id: runB})})'>
                    <div style="font-size: 13px;">${escapeHtml(r.test_name)}</div>
                    <div style="font-size: 11px; color: var(--text-secondary); margin-top: 4px;">
                        Runtime: ${r.runtimeA ? r.runtimeA.toFixed(2) + 's' : 'N/A'} → ${r.runtimeB ? r.runtimeB.toFixed(2) + 's' : 'N/A'}
                    </div>
                </div>`
            ).join('') || '<div style="color: var(--text-secondary); padding: 20px; text-align: center;">No fixed tests</div>';

            document.getElementById('regressionsList').innerHTML = regressions.map(r =>
                `<div style="padding: 8px; border-bottom: 1px solid var(--border-color); cursor: pointer;" onclick='showTestDetails(${JSON.stringify({test_name: r.test_name, state: "failed", runtime: r.runtimeB, run_id: runB})})'>
                    <div style="font-size: 13px;">${escapeHtml(r.test_name)}</div>
                    <div style="font-size: 11px; color: var(--text-secondary); margin-top: 4px;">
                        Flakiness: ${r.flakiness_score ? r.flakiness_score.toFixed(0) + '% stable' : 'N/A'} | Fails: ${r.fail_count}/${r.total_runs}
                    </div>
                </div>`
            ).join('') || '<div style="color: var(--text-secondary); padding: 20px; text-align: center;">No regressions detected</div>';
        }

        // Initialize on load
        document.addEventListener('DOMContentLoaded', function() {
            updateRunInfo();
        });
    """

# --- Report Generator ---

class ReportGenerator:
    def __init__(self, results_dir: Path):
        self.results_dir = results_dir
        self.runs: List[TestRun] = []
        self.pivot_data: List[PivotRow] = []
        self.all_labels: Set[str] = set()

    def parse_results(self):
        """Parse all test run results from the results directory."""
        if not self.results_dir.exists():
            return

        for run_dir in sorted(self.results_dir.glob("run-*")):
            if not run_dir.is_dir():
                continue

            metadata_file = run_dir / "artifacts" / "metadata.json"
            report_file = run_dir / "reports" / "report.json"

            if not metadata_file.exists() or not report_file.exists():
                continue

            try:
                with open(metadata_file, 'r') as f:
                    metadata = json.load(f)
                with open(report_file, 'r') as f:
                    report = json.load(f)
            except Exception as e:
                print(f"Error reading {run_dir}: {e}")
                continue

            # Parse tests
            tests = []
            spec_reports = report[0].get('SpecReports', []) if isinstance(report, list) else []

            # Build artifact index once per run for O(1) lookup
            artifact_index = self._build_artifact_index(run_dir)

            for spec in spec_reports:
                if not spec.get('LeafNodeText'):
                    continue

                hierarchy = spec.get('ContainerHierarchyTexts', [])
                leaf_text = spec['LeafNodeText']
                full_name = ' '.join(hierarchy + [leaf_text]) if hierarchy else leaf_text

                # Collect labels
                labels = []
                for l_list in spec.get('ContainerHierarchyLabels', []):
                    if isinstance(l_list, list): labels.extend(l_list)
                if spec.get('LeafNodeLabels'): labels.extend(spec.get('LeafNodeLabels'))
                self.all_labels.update(labels)

                # Find artifacts using pre-built index
                artifact_meta = self._find_artifacts(artifact_index, full_name)

                tests.append(TestResult(
                    name=full_name,
                    full_name=full_name,
                    leaf_text=leaf_text,
                    state=spec['State'],
                    runtime=spec['RunTime'] / 1e9,
                    failure_message=spec.get('FailureMessage', ''),
                    labels=labels,
                    container_hierarchy=hierarchy,
                    start_time=spec.get('StartTime', ''),
                    artifact_metadata=artifact_meta
                ))

            # Calculate total runtime from tests if not in metadata
            total_runtime = sum(t.runtime for t in tests)

            # Read test output log
            test_output_log = ""
            log_file = run_dir / "reports" / "test-output.log"
            if log_file.exists():
                try:
                    with open(log_file, 'r', encoding='utf-8', errors='replace') as f:
                        lines = f.readlines()
                        if len(lines) > 10000:
                            test_output_log = f"Log truncated. Showing last 10,000 of {len(lines)} lines.\\n\\n" + "".join(lines[-10000:])
                        else:
                            test_output_log = "".join(lines)
                except Exception as e:
                    test_output_log = f"Error reading log file: {e}"

            self.runs.append(TestRun(
                run_id=str(metadata.get('run_id', run_dir.name)),
                start_time=metadata.get('start_time', datetime.now().isoformat()),
                total_tests=metadata.get('total_tests', len(tests)),
                passed_tests=metadata.get('passed_tests', len([t for t in tests if t.state == 'passed'])),
                failed_tests=metadata.get('failed_tests', len([t for t in tests if t.state == 'failed'])),
                environment=metadata.get('environment', {}),
                total_runtime=total_runtime,
                test_output_log=test_output_log,
                tests=tests,
                git_commit=metadata.get('git_commit', ''),
                git_branch=metadata.get('git_branch', ''),
                git_dirty=metadata.get('git_dirty', '')
            ))

        # Sort runs by time (newest first)
        self.runs.sort(key=lambda r: r.start_time, reverse=True)
        self._build_pivot_data()

    def _build_artifact_index(self, run_dir: Path) -> Dict[str, Path]:
        """Build name -> artifact_dir mapping for O(1) lookup."""
        index = {}
        artifacts_dir = run_dir / "artifacts"
        if not artifacts_dir.exists():
            return index
        for artifact_dir in artifacts_dir.glob("*"):
            if not artifact_dir.is_dir():
                continue
            meta_file = artifact_dir / "metadata.json"
            if meta_file.exists():
                try:
                    with open(meta_file) as f:
                        data = json.load(f)
                        if name := data.get('name'):
                            index[name.strip()] = artifact_dir
                except:
                    pass
        return index

    def _find_artifacts(self, artifact_index: Dict[str, Path], test_name: str) -> Optional[Dict[str, Any]]:
        """Locate artifact metadata for a specific test using pre-built index."""
        artifact_dir = artifact_index.get(test_name.strip())
        if not artifact_dir:
            return None

        meta_file = artifact_dir / "metadata.json"
        if not meta_file.exists():
            return None

        try:
            with open(meta_file) as f:
                data = json.load(f)

            data['relative_path'] = str(artifact_dir.relative_to(self.results_dir))
            data['file_contents'] = {}

            # Read log files (last 500 lines)
            for log in data.get('artifacts', {}).get('log_files', []):
                lp = artifact_dir / log
                if lp.exists():
                    try:
                        with open(lp) as lf:
                            lines = lf.readlines()
                            content = ''.join(lines[-500:])
                            data['file_contents'][log] = {
                                'content': content,
                                'type': 'log',
                                'truncated': len(lines) > 500,
                                'total_lines': len(lines)
                            }
                    except Exception as e:
                        data['file_contents'][log] = {'content': str(e), 'type': 'error'}

            # Read resource files (limit 50KB)
            for res in data.get('artifacts', {}).get('resource_files', [])[:10]:
                rp = artifact_dir / res
                if rp.exists():
                    try:
                        with open(rp) as rf:
                            content = rf.read()
                            if len(content) > 51200: content = content[:51200] + '\\n... (truncated)'
                            data['file_contents'][res] = {'content': content, 'type': 'resource'}
                    except Exception as e:
                        data['file_contents'][res] = {'content': str(e), 'type': 'error'}

            # Read event files
            for evt in data.get('artifacts', {}).get('event_files', []):
                ep = artifact_dir / evt
                if ep.exists():
                    try:
                        with open(ep) as ef:
                            data['file_contents'][evt] = {'content': ef.read(), 'type': 'events'}
                    except Exception as e:
                        data['file_contents'][evt] = {'content': str(e), 'type': 'error'}

            return data
        except:
            pass
        return None

    def _build_pivot_data(self):
        """Build the pivot table data structure and analyze flakiness."""
        all_names = sorted({t.full_name for run in self.runs for t in run.tests})

        for name in all_names:
            # Get container hierarchy from first occurrence of test
            test_obj = None
            for run in self.runs:
                test_obj = next((t for t in run.tests if t.full_name == name), None)
                if test_obj:
                    break

            row = PivotRow(
                test_name=name,
                full_test_name=name,
                leaf_text=test_obj.leaf_text if test_obj else name.split(' ')[-1],
                container_hierarchy=test_obj.container_hierarchy if test_obj else []
            )
            
            results_sequence = []
            
            for run in self.runs:
                result = next((t for t in run.tests if t.full_name == name), None)
                if result:
                    row.runs[run.run_id] = {
                        'state': result.state,
                        'runtime': result.runtime,
                        'failure_message': result.failure_message,
                        'labels': result.labels,
                        'artifact_metadata': result.artifact_metadata
                    }
                    row.total_runs += 1
                    row.total_runtime += result.runtime
                    row.min_runtime = min(row.min_runtime, result.runtime)
                    row.max_runtime = max(row.max_runtime, result.runtime)
                    
                    if result.state == 'passed': 
                        row.pass_count += 1
                        results_sequence.append('P')
                    elif result.state == 'failed': 
                        row.fail_count += 1
                        results_sequence.append('F')
                    else: 
                        row.skip_count += 1
                        results_sequence.append('S')
                else:
                    row.runs[run.run_id] = None
            
            if row.total_runs > 0:
                row.pass_rate = (row.pass_count / row.total_runs) * 100
                row.avg_runtime = row.total_runtime / row.total_runs
            
            # Flakiness Analysis
            if row.total_runs >= 2 and row.pass_count > 0 and row.fail_count > 0:
                row.is_flaky = True
                row.flakiness_score = 100 - 2 * abs(row.pass_rate - 50)
                
                # Pattern detection
                if len(results_sequence) >= 3:
                    is_alternating = True
                    for i in range(len(results_sequence) - 1):
                        if results_sequence[i] == results_sequence[i+1]:
                            is_alternating = False
                            break
                    if is_alternating: row.flakiness_pattern = 'alternating'
            
            self.pivot_data.append(row)
            
        # Sort by failure count
        self.pivot_data.sort(key=lambda x: (x.fail_count, -x.pass_rate), reverse=True)

    def _generate_chart_data(self) -> str:
        """Generate JSON data for Chart.js."""
        chronological_runs = sorted(self.runs, key=lambda r: r.start_time)
        labels = [f"Run {r.run_id}" for r in chronological_runs]

        pass_rates = []
        durations = []
        runs_data = []

        for r in chronological_runs:
            rate = (r.passed_tests / r.total_tests * 100) if r.total_tests > 0 else 0
            pass_rates.append(round(rate, 1))
            durations.append(round(r.total_runtime, 1))

            # Add run metadata for comparison
            runs_data.append({
                'run_id': r.run_id,
                'timestamp': r.start_time,
                'total': r.total_tests,
                'passed': r.passed_tests,
                'failed': r.failed_tests,
                'runtime': round(r.total_runtime, 1)
            })

        data = {
            "passRate": {
                "labels": labels,
                "datasets": [{
                    "label": "Pass Rate (%)",
                    "data": pass_rates,
                    "borderColor": "#10b981",
                    "backgroundColor": "rgba(16, 185, 129, 0.1)",
                    "fill": True
                }]
            },
            "duration": {
                "labels": labels,
                "datasets": [{
                    "label": "Total Duration (s)",
                    "data": durations,
                    "borderColor": "#3b82f6",
                    "backgroundColor": "rgba(59, 130, 246, 0.1)",
                    "fill": True
                }]
            },
            "runs": runs_data
        }
        return json.dumps(data)

    def generate_html(self, output_file: Path):
        """Generate the full HTML report."""
        
        # Serialize pivot data for JS
        pivot_json = json.dumps([
            {
                'test_name': r.test_name,
                'is_flaky': r.is_flaky,
                'pass_count': r.pass_count,
                'fail_count': r.fail_count,
                'total_runs': r.total_runs,
                'flakiness_score': r.flakiness_score,
                'runs': r.runs
            } for r in self.pivot_data
        ], default=str)
        
        html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>E2E Test Report</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>{ReportTemplates.CSS}</style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div>
                <h1>E2E Test Results</h1>
                <p style="color: var(--text-secondary)">Generated on {datetime.now().strftime('%Y-%m-%d %H:%M')}</p>
            </div>
            <div style="display: flex; gap: 10px;">
                <button id="themeToggle" class="theme-toggle" onclick="toggleTheme()" title="Toggle theme"></button>
            </div>
        </div>

        <div class="tabs">
            <button class="tab-btn active" onclick="switchTab('dashboard')">Dashboard</button>
            <button class="tab-btn" onclick="switchTab('matrix')">Test Matrix</button>
            {f'<button class="tab-btn" onclick="switchTab(\'comparison\')">Comparison</button>' if len(self.runs) >= 2 else ''}
        </div>

        <!-- Dashboard Tab -->
        <div id="dashboard" class="tab-content active">
            <div class="summary-grid">
                <div class="card" style="border-left: 4px solid #3b82f6;">
                    <div class="label">Total Runs</div>
                    <div class="value" style="color: #3b82f6;">{len(self.runs)}</div>
                </div>
                <div class="card" style="border-left: 4px solid #06b6d4;">
                    <div class="label">Total Tests</div>
                    <div class="value" style="color: #06b6d4;">{len(self.pivot_data)}</div>
                </div>
                <div class="card tooltip" style="border-left: 4px solid #f59e0b;">
                    <div class="label">Flaky Tests</div>
                    <div class="value" style="color: #f59e0b;">{len([r for r in self.pivot_data if r.is_flaky])}</div>
                    <span class="tooltiptext">Tests that show inconsistent results across runs - sometimes passing, sometimes failing. These may indicate timing issues, race conditions, or environmental dependencies.</span>
                </div>
                <div class="card tooltip" style="border-left: 4px solid #ef4444;">
                    <div class="label">Always Failing</div>
                    <div class="value" style="color: #ef4444;">{len([r for r in self.pivot_data if r.fail_count == r.total_runs and r.total_runs > 0])}</div>
                    <span class="tooltiptext">Tests that failed in every single run. These are consistently broken and require immediate attention.</span>
                </div>
                <div class="card tooltip" style="border-left: 4px solid #8b5cf6;">
                    <div class="label">Avg Runtime</div>
                    <div class="value" style="color: #8b5cf6;">{format_duration(sum(r.total_runtime for r in self.runs) / len(self.runs)) if self.runs else 'N/A'}</div>
                    <span class="tooltiptext">Average total runtime across all test runs. Helps track performance trends over time.</span>
                </div>
                <div class="card tooltip" style="border-left: 4px solid #10b981;">
                    <div class="label">Pass Rate Trend</div>
                    <div class="value">
                        {self._get_pass_rate_trend()}
                    </div>
                    <span class="tooltiptext">Pass rate change compared to the previous run. ↑ indicates improvement, ↓ indicates more failures, → means stable.</span>
                </div>

                <!-- Latest Run Summary Card -->
                <div class="card" style="grid-column: span 2;">
                    <div class="label">Latest Run Summary</div>
                    <div style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-top: 12px; font-size: 13px;">
                        <div style="text-align: center;">
                            <div style="color: var(--text-secondary); font-size: 11px; margin-bottom: 4px;">Pass Rate</div>
                            <div style="font-size: 20px; font-weight: 600; color: #10b981;">{self._get_latest_pass_rate()}%</div>
                        </div>
                        <div style="text-align: center;">
                            <div style="color: var(--text-secondary); font-size: 11px; margin-bottom: 4px;">Failures</div>
                            <div style="font-size: 20px; font-weight: 600; color: #ef4444;">{self.runs[0].failed_tests if self.runs else 0}</div>
                        </div>
                        <div style="text-align: center;">
                            <div style="color: var(--text-secondary); font-size: 11px; margin-bottom: 4px;">Runtime</div>
                            <div style="font-size: 20px; font-weight: 600; color: var(--text-primary);">{format_duration(self.runs[0].total_runtime) if self.runs else 'N/A'}</div>
                        </div>
                    </div>
                    <div style="margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--border-color); display: flex; align-items: center; justify-content: center; gap: 8px; font-size: 12px; flex-wrap: wrap;">
                        <code style="background: var(--bg-secondary); padding: 3px 8px; border-radius: 4px; font-size: 11px;">
                            {self.runs[0].git_branch if self.runs and self.runs[0].git_branch else 'unknown'}
                        </code>
                        {f'<span style="color: #ef4444; font-size: 14px;" title="Working directory has uncommitted changes">●</span>' if self.runs and self.runs[0].git_dirty else ''}
                        <code style="background: var(--bg-secondary); padding: 3px 8px; border-radius: 4px; font-size: 11px; cursor: pointer; transition: all 0.2s;"
                              onclick="copyWithElementFeedback('{self.runs[0].git_commit if self.runs and self.runs[0].git_commit else 'unknown'}', this)"
                              onmouseover="this.style.background='var(--bg-hover)'"
                              onmouseout="this.style.background='var(--bg-secondary)'"
                              title="Click to copy full commit hash: {self.runs[0].git_commit if self.runs and self.runs[0].git_commit else 'unknown'}">
                            {svg_icon('copy', 12)} {self.runs[0].git_commit[:7] if self.runs and self.runs[0].git_commit else 'unknown'}
                        </code>
                    </div>
                </div>
            </div>
            
            <div class="charts-container">
                <div class="chart-wrapper">
                    <canvas id="passRateChart"></canvas>
                </div>
                <div class="chart-wrapper">
                    <canvas id="durationChart"></canvas>
                </div>
            </div>
            
            <div class="flaky-section">
                <h3>{svg_icon('warning', 18, '#f59e0b')} Flaky Tests Detected</h3>
                <div class="flaky-grid">
                    {''.join(f'''
                    <div class="card flaky-card" onclick="showTestDetails(window.pivotData.find(r => r.test_name === '{r.test_name}'))">
                        <div class="flaky-header">
                            <span class="badge skipped tooltip">{r.flakiness_score:.0f}% Stable
                                <span class="tooltiptext">Stability score based on pass/fail ratio. Lower values indicate more inconsistent behavior.</span>
                            </span>
                            <span class="badge failed tooltip">{r.flakiness_pattern}
                                <span class="tooltiptext">Pattern: {self._get_flakiness_pattern_description(r.flakiness_pattern)}</span>
                            </span>
                        </div>
                        <div class="flaky-name">{r.test_name}</div>
                        <div class="flaky-stats">
                            Pass: {r.pass_count} | Fail: {r.fail_count}
                        </div>
                    </div>
                    ''' for r in self.pivot_data if r.is_flaky) or '<p>No flaky tests detected.</p>'}
                </div>
            </div>
        </div>

        <!-- Matrix Tab -->
        <div id="matrix" class="tab-content">
            <div class="filters">
                <input type="text" id="searchInput" class="filter-input" placeholder="Search tests..." onkeyup="filterTable()">
                <select id="statusFilter" class="filter-input" onchange="filterTable()">
                    <option value="all">All Statuses</option>
                    <option value="passed">Passed</option>
                    <option value="failed">Failed</option>
                </select>
                <select id="stabilityFilter" class="filter-input" onchange="filterTable()">
                    <option value="all">All Stability</option>
                    <option value="flaky">Flaky Only</option>
                    <option value="stable">Stable Only</option>
                    <option value="always-failing">Always Failing</option>
                </select>
                <select id="labelFilter" class="filter-input" onchange="filterTable()">
                    <option value="all">All Labels</option>
                    {''.join(f'<option value="{l}">{l}</option>' for l in sorted(self.all_labels))}
                </select>
                <button class="theme-toggle" onclick="copyAllTestNames()">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align: middle; margin-right: 4px;">
                        <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                    </svg>
                    Copy All Names
                </button>
            </div>
            
            <div class="table-wrapper">
                <table id="resultsTable">
                    <thead>
                        <tr>
                            <th>Test Name</th>
                            <th class="stats-col">Stats</th>
                            {''.join(f'<th class="run-header" onclick=\'showRunDetails({json.dumps({"run_id": r.run_id, "start_time": r.start_time, "total_tests": r.total_tests, "passed_tests": r.passed_tests, "failed_tests": r.failed_tests, "total_runtime": r.total_runtime, "git_commit": r.git_commit, "git_branch": r.git_branch, "git_dirty": r.git_dirty, "test_output_log": r.test_output_log}, default=str).replace("'", "&#39;")})\'>Run {r.run_id}<br><span style="font-size:10px">{r.date_str}<br>{format_duration(r.total_runtime)}</span></th>' for r in self.runs)}
                        </tr>
                    </thead>
                    <tbody>
                        {self._generate_table_rows()}
                    </tbody>
                </table>
            </div>
        </div>
        
        <!-- Comparison Tab -->
        <div id="comparison" class="tab-content">
            <div class="filters">
                <select id="runASelect" class="filter-input" onchange="updateRunInfo()">
                    {''.join(f'<option value="{r.run_id}">Run {r.run_id} - {r.date_str} ({format_duration(r.total_runtime)})</option>' for r in self.runs)}
                </select>
                <select id="runBSelect" class="filter-input" onchange="updateRunInfo()">
                    {''.join(f'<option value="{r.run_id}" {"selected" if i == 1 else ""}>Run {r.run_id} - {r.date_str} ({format_duration(r.total_runtime)})</option>' for i, r in enumerate(self.runs))}
                </select>
                <button onclick="compareRuns()" class="theme-toggle">Compare</button>
            </div>

            <!-- Run Info Cards -->
            <div class="comparison-run-info">
                <div class="run-info-card">
                    <h4>Run A</h4>
                    <div id="runAInfo" class="run-info-details"></div>
                </div>
                <div class="run-info-card">
                    <h4>Run B</h4>
                    <div id="runBInfo" class="run-info-details"></div>
                </div>
            </div>

            <!-- Summary Cards -->
            <div id="comparisonSummary" class="summary-grid" style="display: none; margin-top: 20px;">
                <div class="card tooltip">
                    <div class="label">New Failures</div>
                    <div id="newFailuresCount" class="value" style="color: #ef4444;">0</div>
                    <span class="tooltiptext">Tests that passed in Run A but failed in Run B. These represent newly broken functionality.</span>
                </div>
                <div class="card tooltip">
                    <div class="label">Fixed Tests</div>
                    <div id="fixedTestsCount" class="value" style="color: #10b981;">0</div>
                    <span class="tooltiptext">Tests that failed in Run A but passed in Run B. These represent fixes or improvements.</span>
                </div>
                <div class="card tooltip">
                    <div class="label">Regressions</div>
                    <div id="regressionsCount" class="value" style="color: #f59e0b;">0</div>
                    <span class="tooltiptext">Flaky tests that have become consistently failing in Run B. These indicate worsening test stability.</span>
                </div>
                <div class="card tooltip">
                    <div class="label">Runtime Diff</div>
                    <div id="runtimeDiff" class="value">-</div>
                    <span class="tooltiptext">Total runtime difference between Run B and Run A. Positive values mean slower, negative means faster.</span>
                </div>
            </div>

            <div class="comparison-grid">
                <div class="comparison-col">
                    <h4>{svg_icon('error', 16, '#ef4444')} New Failures</h4>
                    <div id="newFailuresList"></div>
                </div>
                <div class="comparison-col">
                    <h4>{svg_icon('info', 16, '#10b981')} Fixed Tests</h4>
                    <div id="fixedTestsList"></div>
                </div>
                <div class="comparison-col">
                    <h4>{svg_icon('warning', 16, '#f59e0b')} Regressions</h4>
                    <div id="regressionsList"></div>
                </div>
            </div>
        </div>
    </div>

    <!-- Modal -->
    <div id="testModal" class="modal" onclick="if(event.target === this) closeModal('testModal')">
        <div class="modal-content">
            <div class="modal-header">
                <h2 id="testModalTitle">Test Details</h2>
                <span class="close-btn" onclick="closeModal('testModal')">&times;</span>
            </div>
            <div class="modal-body" id="modalContent"></div>
        </div>
    </div>

    <script>
        window.chartData = {self._generate_chart_data()};
        window.pivotData = {pivot_json};

        {ReportTemplates.JS}
    </script>
</body>
</html>
"""
        with open(output_file, 'w') as f:
            f.write(html_content)
        print(f"Report generated at: {output_file}")

    def _get_latest_pass_rate(self) -> str:
        if not self.runs: return "0.0"
        latest = self.runs[0]
        if latest.total_tests == 0: return "0.0"
        return f"{(latest.passed_tests / latest.total_tests * 100):.1f}"

    def _get_pass_rate_trend(self) -> str:
        """Get pass rate trend compared to previous run."""
        if len(self.runs) < 2:
            return '<span style="color: var(--text-secondary);">—</span>'

        latest = self.runs[0]
        previous = self.runs[1]

        if latest.total_tests == 0 or previous.total_tests == 0:
            return '<span style="color: var(--text-secondary);">—</span>'

        latest_rate = (latest.passed_tests / latest.total_tests * 100)
        previous_rate = (previous.passed_tests / previous.total_tests * 100)
        diff = latest_rate - previous_rate

        if abs(diff) < 0.1:
            return '<span style="color: var(--text-secondary);">→ Stable</span>'
        elif diff > 0:
            return f'<span style="color: #10b981;">↑ +{diff:.1f}%</span>'
        else:
            return f'<span style="color: #ef4444;">↓ {diff:.1f}%</span>'

    def _get_flakiness_pattern_description(self, pattern: str) -> str:
        """Get description for flakiness pattern."""
        descriptions = {
            'intermittent': 'Fails randomly with no clear pattern',
            'occasional': 'Fails infrequently, mostly passes',
            'frequent': 'Fails often, passes sometimes',
            'alternating': 'Alternates between pass and fail',
            'unstable': 'Highly unpredictable behavior'
        }
        return descriptions.get(pattern, 'Unknown pattern')

    def _generate_hierarchical_name(self, container_hierarchy: List[str], leaf_text: str) -> str:
        """Generate breadcrumb-style HTML representation of test name."""
        if not container_hierarchy:
            # No hierarchy - just show the leaf text
            return f'<div class="test-breadcrumb"><span class="breadcrumb-leaf">{html.escape(leaf_text)}</span></div>'

        # Build breadcrumb structure
        parts = []

        # Add container hierarchy items
        for i, container in enumerate(container_hierarchy):
            level_class = f'level-{i}' if i == 0 else ''
            parts.append(f'<span class="breadcrumb-item"><span class="breadcrumb-container {level_class}">{html.escape(container)}</span></span>')
            parts.append('<span class="breadcrumb-separator">›</span>')

        # Add leaf (actual test name)
        parts.append(f'<span class="breadcrumb-item"><span class="breadcrumb-leaf">{html.escape(leaf_text)}</span></span>')

        return f'<div class="test-breadcrumb">{"".join(parts)}</div>'

    def _generate_table_rows(self) -> str:
        rows = []
        for row in self.pivot_data:
            cells = []

            # Test Name with hierarchy
            hierarchical_name = self._generate_hierarchical_name(row.container_hierarchy, row.leaf_text)
            # Full test name for copying (breadcrumb style)
            full_test_name = ' › '.join(row.container_hierarchy + [row.leaf_text]) if row.container_hierarchy else row.leaf_text
            cells.append(f'''<td class="test-name-cell">
                <div class="test-name-wrapper">
                    {hierarchical_name}
                    <button class="copy-test-name-btn" onclick="copyTestName(this)" data-test-name="{html.escape(full_test_name)}" title="Copy test name">
                        {svg_icon('copy', 14)} Copy
                    </button>
                </div>
            </td>''')

            # Stats Cell
            rate_class = 'rate-high' if row.pass_rate >= 90 else 'rate-medium' if row.pass_rate >= 50 else 'rate-low'
            cells.append(f'''
                <td class="stats-cell">
                    <div class="pass-rate {rate_class} tooltip">
                        <span class="rate-value">{row.pass_rate:.1f}%</span>
                        <span class="tooltiptext">Pass rate: {row.pass_count} passed out of {row.total_runs} total runs</span>
                    </div>
                    <div class="counts">{row.pass_count}✓ {row.fail_count}✗ / {row.total_runs}</div>
                    <div class="avg-time">avg: {row.avg_runtime:.1f}s</div>
                </td>
            ''')
            
            for run in self.runs:
                result = row.runs.get(run.run_id)
                if result:
                    # Runtime color
                    rt = result['runtime']
                    rt_class = 'runtime-fast' if rt < 10 else 'runtime-medium' if rt < 30 else 'runtime-slow'
                    
                    # Serialize result for modal
                    data_json = json.dumps({
                        'test_name': row.test_name,
                        'state': result['state'],
                        'runtime': result['runtime'],
                        'run_id': run.run_id,
                        'failure_message': result['failure_message'],
                        'artifact_metadata': result['artifact_metadata']
                    }).replace("'", "&#39;")
                    
                    cells.append(f'''
                        <td class="result-cell" onclick='showTestDetails({data_json})'>
                            <span class="badge {result["state"]}">{result["state"]}</span>
                            <span class="runtime {rt_class}">{rt:.1f}s</span>
                        </td>
                    ''')
                else:
                    cells.append('<td>-</td>')
            
            # Add metadata for filtering
            row_meta = json.dumps({
                'test_name': row.test_name,
                'is_flaky': row.is_flaky,
                'pass_count': row.pass_count,
                'fail_count': row.fail_count,
                'runs': row.runs
            }, default=str).replace('"', '&quot;')
            
            rows.append(f'<tr data-json="{row_meta}">{"".join(cells)}</tr>')
        return "\n".join(rows)

def main():
    if len(sys.argv) > 1:
        results_dir = Path(sys.argv[1])
    else:
        results_dir = Path.cwd()
    
    output_file = results_dir / "test_results_report.html"
    
    print(f"Scanning {results_dir}...")
    generator = ReportGenerator(results_dir)
    generator.parse_results()
    
    if not generator.runs:
        print("No runs found.")
        return

    generator.generate_html(output_file)

if __name__ == "__main__":
    main()
