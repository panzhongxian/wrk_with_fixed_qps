-- 设置请求方法为POST
wrk.method = "POST"
-- 设置请求头
wrk.headers["Content-Type"] = "application/json"

-- 请求体
local body = '{"delay_ms": 100}'

-- 初始化函数
function init(args)
    -- 可以在这里设置一些初始化参数
end

-- 请求函数
function request()
    return wrk.format(nil, nil, nil, body)
end

-- 响应函数
function response(status, headers, body)
    -- 可以在这里处理响应
end 