export function isValidBVID(bvid) {
    return /^BV[0-9A-Za-z]{10,}$/.test(bvid);
}

export function getIssueTypeName(type) {
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
        'parent_not_exist': '父评论不存在（子评论指向不存在的父评论）',
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

export function getSeverityName(severity) {
    const severityNames = {
        'critical': '严重',
        'high': '高',
        'medium': '中',
        'low': '低',
        'info': '信息'
    };
    return severityNames[severity] || severity;
}

export function getCategoryName(category) {
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