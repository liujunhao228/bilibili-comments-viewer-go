import { escapeHtml, processImageSrc } from './utils.js';

export function renderVideoList(videos, VIDEO_LIST, handleVideoClick) {
    VIDEO_LIST.innerHTML = '';
    videos.forEach(video => {
        const card = document.createElement('div');
        card.className = 'video-card';
        card.dataset.bvid = video.bvid;
        card.dataset.title = video.title;
        card.dataset.cover = video.cover;
        card.innerHTML = `
            <div class="video-card-image-container">
                <img src="${processImageSrc(video.cover)}"
                     alt="${escapeHtml(video.title)}"
                     loading="lazy"
                     onerror="this.src='/static/images/default-cover.jpg'">
            </div>
            <div class="video-card-info">
                <h4>${escapeHtml(video.title)}</h4>
                <p class="video-bvid">BV号: ${escapeHtml(video.bvid)}</p>
                <p>评论数: ${video.comment_count || 0}</p>
            </div>
        `;
        card.addEventListener('click', handleVideoClick);
        VIDEO_LIST.appendChild(card);
    });
}

export function renderCommentList(comments, COMMENT_LIST, createCommentElement) {
    COMMENT_LIST.innerHTML = '';
    comments.forEach(comment => {
        const commentEl = createCommentElement(comment);
        COMMENT_LIST.appendChild(commentEl);
    });
}

export function createCommentElement(comment, renderPictures, renderReplies, handleLoadMoreReplies, createReplyElement, escapeHtml) {
    const div = document.createElement('div');
    div.className = 'comment';
    div.dataset.commentId = comment.unique_id;
    const picturesHTML = renderPictures(comment.pictures);
    const headerHTML = `
        <div class="comment-header">
            <span class="comment-user">${escapeHtml(comment.upname)}</span>
            <span class="comment-level">Lv.${comment.level}</span>
            ${comment.sex ? `<span class="comment-sex">(${escapeHtml(comment.sex)})</span>` : ''}
            <div class="comment-meta">
                ${comment.location ? `<span class="location">${escapeHtml(comment.location)}</span>` : ''}
                <span class="like-count">${comment.like_count}</span>
                <span class="comment-time">${escapeHtml(comment.formatted_time)}</span>
            </div>
        </div>
    `;
    const contentHTML = `
        <div class="comment-content">${escapeHtml(comment.content)}</div>
    `;
    let repliesHTML = '';
    if (comment.loadedReplies && comment.loadedReplies.length > 0) {
        repliesHTML = `
            <div class="replies">
                ${renderReplies(comment.loadedReplies, renderPictures, escapeHtml)}
            </div>
        `;
    }
    let loadMoreHTML = '';
    if (comment.totalReplies > (comment.loadedReplies?.length || 0)) {
        loadMoreHTML = `
            <div class="replies-pagination">
                <button class="load-more-replies" 
                        data-comment-id="${comment.unique_id}"
                        data-next-page="2">
                    加载更多回复 (${comment.loadedReplies?.length || 0}/${comment.totalReplies})
                </button>
            </div>
        `;
    }
    div.innerHTML = `
        ${headerHTML}
        ${contentHTML}
        ${picturesHTML}
        ${repliesHTML}
        ${loadMoreHTML}
    `;
    const loadMoreBtn = div.querySelector('.load-more-replies');
    if (loadMoreBtn) {
        loadMoreBtn.addEventListener('click', handleLoadMoreReplies);
    }
    return div;
}

export function renderReplies(replies, renderPictures, escapeHtml, bvid) {
    if (!replies || replies.length === 0) return '';
    return replies.map(reply => {
        const picturesHTML = renderPictures(reply.pictures, processImageSrc, bvid);
        return `
            <div class="reply">
                <div class="comment-header">
                    <span class="comment-user">${escapeHtml(reply.upname)}</span>
                    <span class="comment-level">Lv.${reply.level}</span>
                    ${reply.sex ? `<span class="comment-sex">(${escapeHtml(reply.sex)})</span>` : ''}
                    <div class="comment-meta">
                        ${reply.location ? `<span class="location">${escapeHtml(reply.location)}</span>` : ''}
                        <span class="like-count">${reply.like_count}</span>
                        <span class="comment-time">${escapeHtml(reply.formatted_time)}</span>
                    </div>
                </div>
                <div class="comment-content">${escapeHtml(reply.content)}</div>
                ${picturesHTML}
            </div>
        `;
    }).join('');
}

export function renderPictures(pictures, processImageSrc, bvid) {
    if (!pictures || pictures.length === 0) return '';
    const imgs = pictures.map(pic => {
        if (!pic || !pic.img_src) return '';
        const processedSrc = processImageSrc(pic.img_src, bvid);
        const onerror = `handleImageError(this, '${escapeHtml(pic.img_src)}')`;
        return `<img src="${processedSrc}" alt="评论图片" loading="lazy" onerror="${onerror}">`;
    }).join('');
    return `<div class="comment-pictures">${imgs}</div>`;
}

// 全局图片加载错误处理
if (typeof window !== 'undefined') {
    window.handleImageError = function(imgElement, originalSrc) {
        // 第一次尝试：使用代理服务加载
        if (!imgElement.dataset.retriedProxy) {
            imgElement.src = `/proxy_image?url=${encodeURIComponent(originalSrc)}`;
            imgElement.dataset.retriedProxy = true;
            return;
        }
        // 第二次尝试：使用备用图片
        imgElement.src = '/static/images/default-cover.jpg';
        imgElement.onerror = null; // 防止循环错误
    };
}

export function renderPagination(container, pagination, fetchFunc, ...args) {
    container.innerHTML = '';
    const { current_page, total_pages, has_next, has_prev } = pagination;
    if (total_pages <= 1) return;
    const prevBtn = createPaginationButton('上一页', !has_prev, () => {
        if (has_prev) fetchFunc(current_page - 1, ...args);
    });
    const nextBtn = createPaginationButton('下一页', !has_next, () => {
        if (has_next) fetchFunc(current_page + 1, ...args);
    });
    const pageInfo = document.createElement('span');
    pageInfo.textContent = `第 ${current_page} 页，共 ${total_pages} 页`;
    container.appendChild(prevBtn);
    container.appendChild(pageInfo);
    container.appendChild(nextBtn);
}

export function createPaginationButton(text, disabled, onClick) {
    const btn = document.createElement('button');
    btn.textContent = text;
    btn.disabled = disabled;
    btn.addEventListener('click', onClick);
    return btn;
} 