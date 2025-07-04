import { showLoading, hideLoading, showError, showSuccess, showResults, setLoadingState } from './ui.js';
import { validateDatabase, validateVideoData, repairDatabase, repairVideoData } from './api.js';
import { isValidBVID, getIssueTypeName, getSeverityName, getCategoryName } from './utils.js';
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
    const issuesList = document.getElementById('issues-list');

    bvidInput.addEventListener('input', function() {
        const bvid = bvidInput.value.trim();
        const valid = isValidBVID(bvid);
        validateVideoBtn.disabled = !valid;
        repairVideoBtn.disabled = !valid;
    });
    bvidInput.addEventListener('keypress', function(e) {
        if (e.key === 'Enter' && !validateVideoBtn.disabled) {
            validateVideoBtn.click();
        }
    });
    validateAllBtn.addEventListener('click', function() {
        validateDatabase();
    });
    repairAllBtn.addEventListener('click', function() {
        if (confirm('确定要修复整个数据库吗？此操作将自动修复所有可修复的问题。')) {
            repairDatabase();
        }
    });
    validateVideoBtn.addEventListener('click', function() {
        const bvid = bvidInput.value.trim();
        if (!isValidBVID(bvid)) {
            showError('请输入正确的 BV 号');
            return;
        }
        validateVideoData(bvid);
    });
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
    // BV号点击事件委托
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