import { fetchData, fetchVideos, fetchComments, fetchCommentReplies } from './api.js';
import { initTheme, applyTheme, toggleTheme } from './theme.js';
import { renderVideoList, renderCommentList, createCommentElement, renderReplies, renderPictures, renderPagination, createPaginationButton } from './render.js';
import { openModal, closeModal, bindModalEvents } from './modal.js';
import { escapeHtml, processImageSrc, showLoading, hideLoading, showError } from './utils.js';
import { bindEvents } from './events.js';

document.addEventListener('DOMContentLoaded', () => {
    const API_BASE_URL = window.location.origin;
    const VIDEO_LIST_VIEW = document.getElementById('video-list-view');
    const COMMENT_VIEW = document.getElementById('comment-view');
    const VIDEO_LIST = document.getElementById('video-list');
    const VIDEO_PAGINATION = document.getElementById('video-pagination');
    const VIDEO_LOADING = document.getElementById('video-loading');
    const VIDEO_ERROR = document.getElementById('video-error');
    const SEARCH_INPUT = document.getElementById('search-input');
    const SEARCH_BUTTON = document.getElementById('search-button');
    const BACK_BUTTON = document.getElementById('back-button');
    const VIDEO_TITLE = document.getElementById('video-title');
    const VIDEO_COVER = document.getElementById('video-cover');
    const COMMENT_COUNT = document.getElementById('comment-count');
    const COMMENT_LIST = document.getElementById('comment-list');
    const COMMENT_PAGINATION = document.getElementById('comment-pagination');
    const COMMENT_LOADING = document.getElementById('comment-loading');
    const COMMENT_ERROR = document.getElementById('comment-error');
    const COMMENT_SEARCH_INPUT = document.getElementById('comment-search-input');
    const COMMENT_SEARCH_BUTTON = document.getElementById('comment-search-button');
    const THEME_TOGGLE = document.getElementById('theme-toggle-button');
    const IMAGE_MODAL = document.getElementById('image-modal');
    const MODAL_IMAGE = document.getElementById('modal-image');
    const MODAL_CLOSE = document.querySelector('.modal-close');

    let currentVideoPage = 1;
    let currentCommentPage = 1;
    const perPage = 12;
    const commentsPerPage = 20;
    const repliesPerPage = 5;
    let currentSearchTerm = '';
    let currentCommentSearchTerm = '';
    let selectedBvid = null;
    let totalVideoPages = 1;
    let totalCommentPages = 1;
    const loadedReplies = new Map();

    // 状态管理函数
    function setCurrentVideoPage(page) { currentVideoPage = page; }
    function setCurrentCommentPage(page) { currentCommentPage = page; }
    function setCurrentSearchTerm(term) { currentSearchTerm = term; }
    function setCurrentCommentSearchTerm(term) { currentCommentSearchTerm = term; }
    function setSelectedBvid(bvid) { selectedBvid = bvid; }
    function setTotalVideoPages(pages) { totalVideoPages = pages; }
    function setTotalCommentPages(pages) { totalCommentPages = pages; }

    // 主题初始化
    initTheme(THEME_TOGGLE, applyTheme, toggleTheme);

    // 绑定模态框事件
    bindModalEvents(MODAL_CLOSE, IMAGE_MODAL, MODAL_IMAGE, closeModal);

    // 事件绑定
    bindEvents({
        BACK_BUTTON,
        SEARCH_BUTTON,
        SEARCH_INPUT,
        COMMENT_SEARCH_BUTTON,
        COMMENT_SEARCH_INPUT,
        VIDEO_COVER,
        COMMENT_LIST,
        VIDEO_LIST_VIEW,
        COMMENT_VIEW,
        VIDEO_LIST,
        VIDEO_PAGINATION,
        VIDEO_LOADING,
        VIDEO_ERROR,
        COMMENT_PAGINATION,
        COMMENT_LOADING,
        COMMENT_ERROR,
        VIDEO_TITLE,
        COMMENT_COUNT,
        MODAL_IMAGE,
        IMAGE_MODAL,
        MODAL_CLOSE,
        THEME_TOGGLE,
        fetchVideos: () => fetchVideos(API_BASE_URL, perPage, VIDEO_LOADING, VIDEO_LIST, VIDEO_ERROR, VIDEO_PAGINATION, fetchData, (videos) => renderVideoList(videos, VIDEO_LIST, handleVideoClick), renderPagination, showLoading, showError, hideLoading, currentVideoPage, currentSearchTerm, setTotalVideoPages, setCurrentVideoPage),
        fetchComments: () => fetchComments(API_BASE_URL, commentsPerPage, COMMENT_LOADING, COMMENT_LIST, COMMENT_ERROR, COMMENT_PAGINATION, fetchData, fetchCommentReplies, (comments) => renderCommentList(comments, COMMENT_LIST, (comment) => createCommentElement(
            comment,
            (pictures) => renderPictures(pictures, processImageSrc, selectedBvid),
            (replies) => renderReplies(replies, renderPictures, escapeHtml, selectedBvid),
            handleLoadMoreReplies,
            (reply) => createReplyElement(reply, (pictures) => renderPictures(pictures, processImageSrc, selectedBvid), escapeHtml),
            escapeHtml
        )), renderPagination, showLoading, showError, hideLoading, selectedBvid, currentCommentPage, currentCommentSearchTerm, setTotalCommentPages, setCurrentCommentPage, COMMENT_COUNT),
        handleVideoClick,
        handleLoadMoreReplies,
        openModal: (url) => openModal(url, MODAL_IMAGE, IMAGE_MODAL),
        closeModal: () => closeModal(MODAL_IMAGE, IMAGE_MODAL),
        loadedReplies,
        setCurrentVideoPage,
        setCurrentCommentPage,
        setCurrentSearchTerm,
        setCurrentCommentSearchTerm,
        setSelectedBvid,
        setTotalVideoPages,
        setTotalCommentPages
    });

    // 入口初始化
    fetchVideos(API_BASE_URL, perPage, VIDEO_LOADING, VIDEO_LIST, VIDEO_ERROR, VIDEO_PAGINATION, fetchData, (videos) => renderVideoList(videos, VIDEO_LIST, handleVideoClick), renderPagination, showLoading, showError, hideLoading, currentVideoPage, currentSearchTerm, setTotalVideoPages, setCurrentVideoPage);

    // 处理视频点击
    function handleVideoClick(event) {
        const card = event.currentTarget;
        selectedBvid = card.dataset.bvid;
        const title = card.dataset.title;
        const cover = card.dataset.cover || '';
        VIDEO_LIST_VIEW.style.display = 'none';
        COMMENT_VIEW.style.display = 'block';
        window.scrollTo(0, 0);
        VIDEO_TITLE.innerHTML = `${escapeHtml(title)} <span class="video-bvid-detail">(${escapeHtml(selectedBvid)})</span>`;
        VIDEO_COVER.src = processImageSrc(cover, selectedBvid);
        VIDEO_COVER.alt = escapeHtml(title);
        VIDEO_COVER.onerror = function() {
            this.onerror = null;
            this.src = `/proxy_image?url=${encodeURIComponent(cover)}`;
        };
        COMMENT_COUNT.textContent = '...';
        currentCommentPage = 1;
        currentCommentSearchTerm = '';
        COMMENT_SEARCH_INPUT.value = '';
        loadedReplies.clear();
        fetchComments(API_BASE_URL, commentsPerPage, COMMENT_LOADING, COMMENT_LIST, COMMENT_ERROR, COMMENT_PAGINATION, fetchData, fetchCommentReplies, (comments) => renderCommentList(comments, COMMENT_LIST, (comment) => createCommentElement(
            comment,
            (pictures) => renderPictures(pictures, processImageSrc, selectedBvid),
            (replies) => renderReplies(replies, renderPictures, escapeHtml, selectedBvid),
            handleLoadMoreReplies,
            (reply) => createReplyElement(reply, (pictures) => renderPictures(pictures, processImageSrc, selectedBvid), escapeHtml),
            escapeHtml
        )), renderPagination, showLoading, showError, hideLoading, selectedBvid, currentCommentPage, currentCommentSearchTerm, setTotalCommentPages, setCurrentCommentPage, COMMENT_COUNT);
    }

    // 处理加载更多回复
    async function handleLoadMoreReplies(event) {
        const button = event.currentTarget;
        const commentId = button.dataset.commentId;
        const nextPage = parseInt(button.dataset.nextPage);
        button.disabled = true;
        button.textContent = '加载中...';
        try {
            const replyData = await fetchCommentReplies(API_BASE_URL, commentId, nextPage, fetchData, repliesPerPage);
            if (replyData && replyData.replies.length > 0) {
                const commentEl = document.querySelector(`.comment[data-comment-id="${commentId}"]`);
                const repliesContainer = commentEl.querySelector('.replies');
                replyData.replies.forEach(reply => {
                    repliesContainer.innerHTML += createReplyElement(reply, (pictures) => renderPictures(pictures, processImageSrc, selectedBvid), escapeHtml);
                });
                const totalLoaded = (nextPage - 1) * repliesPerPage + replyData.replies.length;
                button.dataset.nextPage = nextPage + 1;
                if (totalLoaded < replyData.total) {
                    button.textContent = `加载更多回复 (${totalLoaded}/${replyData.total})`;
                    button.disabled = false;
                } else {
                    button.remove();
                }
            } else {
                button.textContent = '没有更多回复';
            }
        } catch (error) {
            console.error('加载更多回复失败:', error);
            button.textContent = '加载失败，点击重试';
            button.disabled = false;
        }
    }

    // 创建回复元素
    function createReplyElement(reply, renderPictures, escapeHtml) {
        const picturesHTML = renderPictures(reply.pictures, processImageSrc, selectedBvid);
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
    }
});