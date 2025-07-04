import { getCategoryName, getIssueTypeName, getSeverityName } from './utils.js';

export function setLoadingState(isLoading) {
    const validateAllBtn = document.getElementById('validate-all-btn');
    const repairAllBtn = document.getElementById('repair-all-btn');
    const validateVideoBtn = document.getElementById('validate-video-btn');
    const repairVideoBtn = document.getElementById('repair-video-btn');
    const bvidInput = document.getElementById('bvid-input');
    function isValidBVID(bvid) {
        return /^BV[0-9A-Za-z]{10,}$/.test(bvid);
    }
    validateAllBtn.disabled = isLoading;
    repairAllBtn.disabled = isLoading;
    validateVideoBtn.disabled = isLoading || !isValidBVID(bvidInput.value.trim());
    repairVideoBtn.disabled = isLoading || !isValidBVID(bvidInput.value.trim());
}

export function showLoading(message) {
    const loading = document.getElementById('loading');
    const error = document.getElementById('error');
    const success = document.getElementById('success');
    const results = document.getElementById('results');
    loading.textContent = message;
    loading.style.display = 'block';
    error.style.display = 'none';
    success.style.display = 'none';
    results.style.display = 'none';
    setLoadingState(true);
}

export function hideLoading() {
    const loading = document.getElementById('loading');
    loading.style.display = 'none';
    setLoadingState(false);
}

export function showError(message) {
    const error = document.getElementById('error');
    const success = document.getElementById('success');
    error.textContent = message;
    error.style.display = 'block';
    success.style.display = 'none';
    setLoadingState(false);
    window.scrollTo({top: error.offsetTop-30, behavior: 'smooth'});
}

export function showSuccess(message) {
    const error = document.getElementById('error');
    const success = document.getElementById('success');
    success.textContent = message;
    success.style.display = 'block';
    error.style.display = 'none';
    setLoadingState(false);
    window.scrollTo({top: success.offsetTop-30, behavior: 'smooth'});
}

export function showResults(result) {
    const summary = document.getElementById('summary');
    const issuesList = document.getElementById('issues-list');
    const results = document.getElementById('results');
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