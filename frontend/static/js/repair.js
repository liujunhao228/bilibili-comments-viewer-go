// 此文件已拆分为 frontend/static/js/repair/ 目录下的多个模块。
// 入口请使用 repair/index.js

import { initTheme, applyTheme, toggleTheme } from '/static/js/theme.js';
document.addEventListener('DOMContentLoaded', function() {
    // 主题切换初始化
    const THEME_TOGGLE = document.getElementById('theme-toggle-button');
    initTheme(THEME_TOGGLE, applyTheme, toggleTheme);
    const validateAllBtn = document.getElementById('validate-all-btn');
    const repairAllBtn = document.getElementById('repair-all-btn');
    const validateVideoBtn = document.getElementById('validate-video-btn');
    const repairVideoBtn = document.getElementById('repair-video-btn');
    const bvidInput = document.getElementById('bvid-input');
    const loading = document.getElementById('loading');
    const error = document.getElementById('error');
    const success = document.getElementById('success');
    const results = document.getElementById('results');
    const summary = document.getElementById('summary');
    const issuesList = document.getElementById('issues-list');

    // BV号输入校验
    function isValidBVID(bvid) {
        return /^BV[0-9A-Za-z]{10,}$/.test(bvid);
    }
    bvidInput.addEventListener('input', function() {
        const bvid = bvidInput.value.trim();
        const valid = isValidBVID(bvid);
        validateVideoBtn.disabled = !valid;
        repairVideoBtn.disabled = !valid;
    });
    // 回车键校验视频
    bvidInput.addEventListener('keypress', function(e) {
        if (e.key === 'Enter' && !validateVideoBtn.disabled) {
            validateVideoBtn.click();
        }
    });
    // 全库校验
    validateAllBtn.addEventListener('click', function() {
        validateDatabase();
    });
    // 全库修复
    repairAllBtn.addEventListener('click', function() {
        if (confirm('确定要修复整个数据库吗？此操作将自动修复所有可修复的问题。')) {
            repairDatabase();
        }
    });
    // 校验单视频
    validateVideoBtn.addEventListener('click', function() {
        const bvid = bvidInput.value.trim();
        if (!isValidBVID(bvid)) {
            showError('请输入正确的 BV 号');
            return;
        }
        validateVideoData(bvid);
    });
    // 修复单视频
    repairVideoBtn.addEventListener('click', function() {
        const bvid = bvidInput.value.trim();
        if (!isValidBVID(bvid)) {
            showError('请输入正确的 BV 号');
            return;
        }
        if (confirm(`确定要修复视频 ${bvid} 的数据吗？此操作将自动修复该视频的所有可修复问题。`)) {
            repairVideoData(bvid);
        }
    });
    function setLoadingState(isLoading) {
        validateAllBtn.disabled = isLoading;
        repairAllBtn.disabled = isLoading;
        validateVideoBtn.disabled = isLoading || !isValidBVID(bvidInput.value.trim());
        repairVideoBtn.disabled = isLoading || !isValidBVID(bvidInput.value.trim());
    }
    function showLoading(message) {
        loading.textContent = message;
        loading.style.display = 'block';
        error.style.display = 'none';
        success.style.display = 'none';
        results.style.display = 'none';
        setLoadingState(true);
    }
    function hideLoading() {
        loading.style.display = 'none';
        setLoadingState(false);
    }
    function showError(message) {
        error.textContent = message;
        error.style.display = 'block';
        success.style.display = 'none';
        setLoadingState(false);
        window.scrollTo({top: error.offsetTop-30, behavior: 'smooth'});
    }
    function showSuccess(message) {
        success.textContent = message;
        success.style.display = 'block';
        error.style.display = 'none';
        setLoadingState(false);
        window.scrollTo({top: success.offsetTop-30, behavior: 'smooth'});
    }
    function showResults(result) {
        // 判断是否为单视频校验
        if (result.video_id) {
            summary.style.display = 'none';
        } else {
            summary.style.display = 'block';
            summary.innerHTML = `
                <div class="summary-title">校验摘要</div>
                <div class="summary-stats">
                    <div class="stat-item">
                        <div class="stat-value">${result.summary.total_videos}</div>
                        <div class="stat-label">视频总数</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-value">${result.summary.total_comments}</div>
                        <div class="stat-label">评论总数</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-value">${result.summary.issues_found}</div>
                        <div class="stat-label">发现问题</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-value">${result.summary.issues_fixed}</div>
                        <div class="stat-label">已修复</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-value">${result.summary.issues_unfixable}</div>
                        <div class="stat-label">无法修复</div>
                    </div>
                </div>
            `;
        }
        if (result.issues && result.issues.length > 0) {
            issuesList.innerHTML = result.issues.map(issue => {
                let bvidInfo = '';
                if (issue.affected_bvids && issue.affected_bvids.length > 0) {
                    const bvidList = issue.affected_bvids.slice(0, 5)
                        .map(bvid => `<a href="#" class="bvid-link" data-bvid="${bvid}">${bvid}</a>`)
                        .join(', ');
                    const moreCount = issue.affected_bvids.length > 5 ? ` 等${issue.affected_bvids.length}个` : '';
                    bvidInfo = `<div class="issue-bvids">受影响的BV号: ${bvidList}${moreCount}</div>`;
                }
                let categoryInfo = '';
                if (issue.category) {
                    categoryInfo = `<div class="issue-category">分类: ${getCategoryName(issue.category)}</div>`;
                }
                return `
                    <div class="issue-card">
                        <div class="issue-header">
                            <div class="issue-type">${getIssueTypeName(issue.type)}</div>
                            <div class="issue-severity severity-${issue.severity}">${getSeverityName(issue.severity)}</div>
                        </div>
                        <div class="issue-description">${issue.description}</div>
                        ${issue.details ? `<div class="issue-details-text">${issue.details}</div>` : ''}
                        ${bvidInfo}
                        ${categoryInfo}
                        <div class="issue-details">
                            <span>影响数量: ${issue.count}</span>
                            <span>可修复: ${issue.fixable ? '是' : '否'}</span>
                            <span>状态: ${issue.fixed ? '已修复' : '未修复'}</span>
                        </div>
                    </div>
                `;
            }).join('');
        } else {
            issuesList.innerHTML = '<p style="text-align: center; color: #666; padding: 20px;">未发现任何问题，数据库状态良好！</p>';
        }
        results.style.display = 'block';
        window.scrollTo({top: results.offsetTop-30, behavior: 'smooth'});
    }
    function validateDatabase() {
        showLoading('正在校验整个数据库...');
        fetch('/api/repair/validate')
            .then(response => response.json())
            .then(data => {
                hideLoading();
                if (data.status === 'success') {
                    showResults(data.result);
                    showSuccess(data.message);
                } else {
                    showError(data.message || '校验失败');
                }
            })
            .catch(err => {
                hideLoading();
                showError('网络错误: ' + err.message);
            });
    }
    function validateVideoData(bvid) {
        showLoading(`正在校验视频 ${bvid} 的数据...`);
        fetch(`/api/repair/validate/${bvid}`)
            .then(response => response.json())
            .then(data => {
                hideLoading();
                if (data.status === 'success') {
                    showResults(data.result);
                    showSuccess(data.message);
                } else {
                    showError(data.message || '校验失败');
                }
            })
            .catch(err => {
                hideLoading();
                showError('网络错误: ' + err.message);
            });
    }
    function repairDatabase() {
        showLoading('正在修复整个数据库...');
        fetch('/api/repair/fix', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        })
            .then(response => response.json())
            .then(data => {
                hideLoading();
                if (data.status === 'success') {
                    showResults(data.result);
                    showSuccess(data.message);
                } else {
                    showError(data.message || '修复失败');
                }
            })
            .catch(err => {
                hideLoading();
                showError('网络错误: ' + err.message);
            });
    }
    function repairVideoData(bvid) {
        showLoading(`正在修复视频 ${bvid} 的数据...`);
        fetch(`/api/repair/fix/${bvid}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        })
            .then(response => response.json())
            .then(data => {
                hideLoading();
                if (data.status === 'success') {
                    showResults(data.result);
                    showSuccess(data.message);
                } else {
                    showError(data.message || '修复失败');
                }
            })
            .catch(err => {
                hideLoading();
                showError('网络错误: ' + err.message);
            });
    }
    function getIssueTypeName(type) {
        const typeNames = {
            'empty_video_title': '空视频标题',
            'duplicate_bvid': '重复BV号',
            'video_missing_comments': '视频缺少评论数据',
            'orphan_comments': '孤立评论',
            'duplicate_comments': '重复评论',
            'empty_comment_content': '空评论内容',
            'invalid_timestamp': '异常时间戳',
            'invalid_parent_reference': '无效父评论引用',
            'invalid_child_reference': '无效子评论引用',
            'self_reference': '自引用关系',
            'inconsistent_stats': '统计不一致',
            'missing_stats': '缺失统计',
            'video_not_found': '视频不存在',
            'data_integrity': '数据完整性错误',
            'data_consistency': '数据一致性错误',
            'data_validation': '数据验证错误',
            'data_relationship': '数据关系错误',
            'system_error': '系统错误',
            'network_error': '网络错误',
            'database_error': '数据库错误',
            'api_error': 'API错误',
            'business_logic': '业务逻辑错误',
            'user_input': '用户输入错误',
            'configuration': '配置错误'
        };
        return typeNames[type] || type;
    }
    function getSeverityName(severity) {
        const severityNames = {
            'critical': '严重',
            'high': '高',
            'medium': '中',
            'low': '低',
            'info': '信息'
        };
        return severityNames[severity] || severity;
    }
    function getCategoryName(category) {
        const categoryNames = {
            'data_integrity': '数据完整性',
            'data_consistency': '数据一致性',
            'data_validation': '数据验证',
            'data_relationship': '数据关系',
            'system_error': '系统错误',
            'network_error': '网络错误',
            'database_error': '数据库错误',
            'api_error': 'API错误',
            'business_logic': '业务逻辑',
            'user_input': '用户输入',
            'configuration': '配置错误'
        };
        return categoryNames[category] || category;
    }
    // 在DOMContentLoaded回调内结尾处添加事件委托
    issuesList.addEventListener('click', function(e) {
        if (e.target.classList.contains('bvid-link')) {
            e.preventDefault();
            const bvid = e.target.dataset.bvid;
            bvidInput.value = bvid;
            bvidInput.dispatchEvent(new Event('input'));
            validateVideoBtn.click();
        }
    });
}); 