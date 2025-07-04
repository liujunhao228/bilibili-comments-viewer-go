import { showLoading, hideLoading, showError, showSuccess, showResults } from './ui.js';

export function validateDatabase() {
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

export function validateVideoData(bvid) {
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

export function repairDatabase() {
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

export function repairVideoData(bvid) {
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