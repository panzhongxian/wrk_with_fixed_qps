import os
import json
import psutil
import subprocess
from flask import Flask, render_template, jsonify, request
from flask_socketio import SocketIO
import threading
import time
import logging
import csv
from datetime import datetime
import pandas as pd
import sys
import signal

# 配置日志
logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)

app = Flask(__name__)
socketio = SocketIO(app, cors_allowed_origins="*", ping_timeout=60, ping_interval=25)  # 增加超时时间和心跳间隔

# 存储压测进程信息
current_process = None
process_output = []
latency_data = []
ws_connected = False
last_stats_position = 0
monitoring_thread = None
is_monitoring = False

@socketio.on('connect')
def handle_connect():
    global ws_connected
    ws_connected = True
    logger.info("WebSocket 客户端已连接")

@socketio.on('disconnect')
def handle_disconnect():
    global ws_connected
    ws_connected = False
    logger.info("WebSocket 客户端已断开连接")

def safe_emit(event, data):
    """安全地发送 WebSocket 消息，如果连接断开则不发送"""
    if ws_connected:
        try:
            socketio.emit(event, data)
        except Exception as e:
            logger.error(f"发送 WebSocket 消息失败: {str(e)}")

def get_process_stats(pid):
    try:
        process = psutil.Process(pid)
        if not process.is_running():
            return None
        return {
            'cpu_percent': process.cpu_percent(),
            'memory_percent': process.memory_percent(),
            'memory_info': process.memory_info().rss / 1024 / 1024  # MB
        }
    except psutil.NoSuchProcess:
        logger.warning(f"进程 {pid} 已不存在")
        return None
    except Exception as e:
        logger.error(f"获取进程状态失败: {str(e)}")
        return None

def monitor_process(pid):
    global is_monitoring
    is_monitoring = True
    while is_monitoring:
        try:
            stats = get_process_stats(pid)
            if stats:
                safe_emit('process_stats', stats)
            else:
                # 如果进程不存在，通知前端并退出监控
                safe_emit('process_ended', {'message': '压测进程已结束'})
                break
            time.sleep(1)
        except Exception as e:
            logger.error(f"监控进程时发生错误: {str(e)}")
            break
    is_monitoring = False

def monitor_stats_file():
    """监控 stats.csv 文件的变化并发送数据到前端"""
    global last_stats_position
    
    while current_process and current_process.poll() is None:
        try:
            if os.path.exists('stats.csv'):
                with open('stats.csv', 'r') as f:
                    # 移动到上次读取的位置
                    f.seek(last_stats_position)
                    
                    # 读取新数据
                    new_data = f.read()
                    if new_data:
                        # 更新文件位置
                        last_stats_position = f.tell()
                        
                        # 解析CSV数据
                        try:
                            df = pd.read_csv('stats.csv')
                            if not df.empty:
                                # 转换时间列为datetime
                                df['时间点'] = pd.to_datetime(df['时间点'])
                                
                                # 准备图表数据
                                chart_data = {
                                    'timestamps': df['时间点'].dt.strftime('%H:%M:%S').tolist(),
                                    'qps': df['当秒请求数'].tolist(),
                                    'errors': df['错误数量'].tolist(),
                                    'avg_latency': df['平均延迟'].tolist(),
                                    'p75_latency': df['p75_latency'].tolist(),
                                    'p90_latency': df['p90_latency'].tolist(),
                                    'p99_latency': df['p99_latency'].tolist()
                                }
                                
                                # 发送数据到前端
                                safe_emit('stats_data', chart_data)
                        except Exception as e:
                            logger.error(f"解析stats.csv数据失败: {str(e)}")
            
            time.sleep(1)  # 每秒检查一次
        except Exception as e:
            logger.error(f"监控stats.csv文件失败: {str(e)}")
            time.sleep(1)

def check_and_build_wrkx():
    """检查并构建wrkx"""
    wrkx_path = os.path.join(os.path.dirname(os.path.dirname(__file__)), 'wrkx', 'wrkx')
    if not os.path.exists(wrkx_path):
        print("wrkx不存在，开始构建...")
        wrkx_dir = os.path.join(os.path.dirname(os.path.dirname(__file__)), 'wrkx')
        try:
            # 切换到wrkx目录
            original_dir = os.getcwd()
            os.chdir(wrkx_dir)
            
            # 执行go build
            result = subprocess.run(['go', 'build'], capture_output=True, text=True)
            
            # 返回原目录
            os.chdir(original_dir)
            
            if result.returncode == 0:
                print("wrkx构建成功")
            else:
                print("wrkx构建失败:", result.stderr)
                sys.exit(1)
        except Exception as e:
            print("构建wrkx时出错:", str(e))
            sys.exit(1)
    else:
        print("wrkx已存在，跳过构建")

def build_command(config):
    """根据配置构建压测命令"""
    base_cmd = ['../wrkx/wrkx']
    
    # 添加目标URL
    base_cmd.extend(['--url', config['targetUrl']])
    
    # 添加压测模式参数（二选一）
    if 'qps' in config and config['qps'] > 0:
        base_cmd.extend(['--qps', str(config['qps'])])
        if 'maxWorkers' in config:
            base_cmd.extend(['--max-workers', str(config['maxWorkers'])])
    else:
        base_cmd.extend(['--concurrency', str(config['concurrency'])])
    
    # 添加通用参数
    base_cmd.extend(['--duration', str(config['duration'])])
    if 'timeout' in config:
        base_cmd.extend(['--timeout', str(config['timeout'])])
    
    # 添加请求来源参数（三选一）
    if 'file' in config and config['file']:
        base_cmd.extend(['--file', config['file']])
        if 'reqTemplate' in config and config['reqTemplate']:
            base_cmd.extend(['--req-template', config['reqTemplate']])
    elif 'requestBody' in config and config['requestBody']:
        base_cmd.extend(['--request', config['requestBody']])
    
    # 添加每秒统计参数
    if config.get('enableSecondStats'):
        base_cmd.append('--enable-second-stats')
    
    logger.info(f"构建的命令: {' '.join(base_cmd)}")
    return base_cmd

@app.route('/')
def index():
    logger.info("访问主页")
    return render_template('index.html')

SAFE_DIRECTORY = '/data/'

def open_safe_file(file_path):
    normalized_path = os.path.normpath(file_path)
    full_path =.path.join(SAFE_DIRECTORY, normalized_path)
    full_path = os.path.abspath(full_path)
    safe_abs_path = os.path.abspath(S_DIRECTORY)
    if not full_path.startswith(safe_abs_path):
        raise ValueError("Invalid file path")
    return open(full_path,r')

@app.route('/api/preview-file', methods=['POST'])
def preview_file():
    try:
        data = request.get_json()
        file_path = data.get('path')
        
        if not file_path or not os.path.exists(file_path):
            return jsonify({'error': '文件不存在'}), 404

        if not file_path.endswith(('.txt', '.log', '.json', '.csv')):
            return jsonify({'error': '不支持的文件格式'}), 400

        with open_safe_file(file_path, 'r') as f:
            content = f.read()
        
        return jsonify({'content': content})
    except Exception as e:
        logger.error(f"预览文件失败: {str(e)}")
        return jsonify({'error': str(e)}), 500

@app.route('/api/preview-csv', methods=['POST'])
def preview_csv():
    try:
        data = request.get_json()
        file_path = data.get('path')

        if not file_path or not os.path.exists(file_path):
            return jsonify({'error': '文件不存在'}), 404

        if not file_path.endswith('.csv'):
            return jsonify({'error': '文件必须是CSV格式'}), 400

        content = []
        with open_safe_file(file_path, 'r') as f:
            reader = csv.reader(f)
            for row in reader:
                content.append(','.join(row))

        return jsonify({'content': '\n'.join(content)})
    except Exception as e:
        logger.error(f"预览CSV文件失败: {str(e)}")
        return jsonify({'error': str(e)}), 500

@app.route('/api/start', methods=['POST'])
def start_test():
    global current_process, process_output, latency_data, last_stats_position, monitoring_thread
    logger.info("开始压测")
    
    try:
        # 如果已有进程在运行，先停止它
        if current_process:
            current_process.terminate()
            current_process = None
        
        # 重置stats.csv文件位置
        last_stats_position = 0
        
        config = request.get_json()
        logger.info(f"压测配置: {config}")
        
        # 构建命令
        command = build_command(config)
        logger.info(f"执行命令: {' '.join(command)}")
        
        process_output = []
        latency_data = []
        
        # 启动压测进程
        current_process = subprocess.Popen(
            command,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            universal_newlines=True,
            bufsize=1
        )
        
        # 等待一小段时间确保进程启动
        time.sleep(0.5)
        
        # 检查进程是否成功启动
        if current_process.poll() is not None:
            error_output = current_process.stdout.read()
            raise Exception(f"进程启动失败: {error_output}")
        
        # 启动监控线程
        monitoring_thread = threading.Thread(target=monitor_process, args=(current_process.pid,), daemon=True)
        monitoring_thread.start()
        
        # 启动stats.csv监控线程
        threading.Thread(target=monitor_stats_file, daemon=True).start()
        
        # 启动输出收集线程
        def collect_output():
            try:
                for line in current_process.stdout:
                    if not line:
                        break
                    process_output.append(line.strip())
                    safe_emit('process_output', {'line': line.strip()})
                    
                    # 解析延迟数据
                    if 'latency' in line.lower():
                        try:
                            latency = float(line.split('latency:')[1].split()[0])
                            latency_data.append(latency)
                            safe_emit('latency_data', {'latency': latency})
                        except Exception as e:
                            logger.error(f"解析延迟数据失败: {str(e)}")
            except Exception as e:
                logger.error(f"收集输出时发生错误: {str(e)}")
            finally:
                # 确保进程结束时通知前端
                if current_process and current_process.poll() is not None:
                    safe_emit('process_ended', {'message': '压测进程已结束'})
        
        threading.Thread(target=collect_output, daemon=True).start()
        
        return jsonify({'status': 'started', 'pid': current_process.pid})
    except Exception as e:
        logger.error(f"启动压测失败: {str(e)}")
        if current_process:
            current_process.terminate()
            current_process = None
        return jsonify({'status': 'error', 'message': str(e)}), 500

@app.route('/api/stop', methods=['POST'])
def stop_test():
    global current_process, monitoring_thread, is_monitoring
    logger.info("停止压测")
    if current_process:
        try:
            # 停止监控
            is_monitoring = False
            if monitoring_thread:
                monitoring_thread.join(timeout=1)
            
            # 终止进程
            current_process.terminate()
            # 等待进程结束
            current_process.wait(timeout=5)
            
            current_process = None
            monitoring_thread = None
            
            return jsonify({'status': 'stopped'})
        except Exception as e:
            logger.error(f"停止进程时发生错误: {str(e)}")
    return jsonify({'status': 'error', 'message': 'No process running'})

@app.route('/api/status', methods=['GET'])
def get_status():
    if current_process:
        return jsonify({
            'running': True,
            'pid': current_process.pid,
            'stats': get_process_stats(current_process.pid)
        })
    return jsonify({'running': False})

if __name__ == '__main__':
    # 检查并构建wrkx
    check_and_build_wrkx()
    
    logger.info("启动服务器在端口 8081")
    socketio.run(app, debug=True, host='0.0.0.0', port=8081)
