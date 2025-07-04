export function escapeHtml(unsafe) {
    if (unsafe == null) {
        return '';
    }
    const str = String(unsafe);
    return str
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
}

export function processImageSrc(src, bvid) {
    if (!src) return '';
    
    // 处理B站图片URL - 映射到本地图片路径
    if (src.includes('hdslb.com')) {
        // 从URL中提取文件名
        const urlParts = src.split('/');
        const filename = urlParts[urlParts.length - 1];
        // 如果有BV号，使用BV号目录；否则使用默认目录
        if (bvid) {
            return `/local_images/${bvid}/${filename}`;
        } else {
            return `/local_images/${filename}`;
        }
    }
    
    // 处理本地图片路径 - 优先匹配BV号目录下的图片
    if (bvid && /^[a-f0-9]{32}\.(jpg|png|jpeg|webp|gif)$/.test(src)) {
        return `/local_images/${bvid}/${src}`;
    }
    // 处理其他本地图片路径
    if (src.startsWith('cover/') || src.startsWith('images/')) {
        return `/local_images/${encodeURIComponent(src)}`;
    }
    // 默认图片路径
    return `/static/images/${encodeURIComponent(src)}`;
}

export function showLoading(indicator, content, error) {
    indicator.style.display = 'block';
    content.innerHTML = '';
    error.style.display = 'none';
    error.textContent = '';
}

export function hideLoading(indicator) {
    indicator.style.display = 'none';
}

export function showError(indicator, content, message) {
    indicator.textContent = message;
    indicator.style.display = 'block';
    content.innerHTML = '';
} 