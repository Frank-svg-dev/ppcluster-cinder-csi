import json
import subprocess

from flask import Flask, jsonify, request
from nova import utils
import nova.conf
from os_brick.initiator import connector as brick_conn
from os_brick import exception as os_brick_exception
from oslo_config import cfg
from nova.volume import cinder
from os_brick import initiator
from nova import context as nova_context
volume_api = cinder.API()

CONF =  nova.conf.CONF

app = Flask(__name__)

# 一个简单的路由，当访问根URL时返回一个欢迎消息
@app.route('/')
def hello():
    return "ok", 200


def get_ecsnode_id():
    ecsnode_json_command = "kubectl get ecsnode -n openstack -ojson"
    ecsnode_result = subprocess.check_output(ecsnode_json_command, shell=True, stderr=subprocess.STDOUT)

    hostname_command = "hostname | awk -F . '{print $1}'"
    hostname_result = subprocess.check_output(hostname_command, shell=True, stderr=subprocess.STDOUT)



    res = json.loads(ecsnode_result)

    print(hostname_result)

    for item in res['items']:
        print(item['metadata']['name'])
        if item['data']['hostname'] == hostname_result:
            return item['metadata']['name']

    return None


@app.route('/ecsnode_id', methods=['GET'])
def get_ecsnode():
    return node_id



@app.route('/connector', methods=['POST'])
def get_connector():
    if request.is_json:
        connector = brick_conn.get_connector_properties(root_helper=utils.get_root_helper,
                                                        my_ip=CONF.my_block_storage_ip, multipath=True,
                                                        enforce_multipath=True, host=CONF.host)
        return connector, 201
    else:
        return jsonify({"error": "Request must be JSON"}), 400

@app.route('/attachvolume', methods=['POST'])
def attach_volume(connection_info):
    if request.is_json:
        data = request.get_json()
        conntest = brick_conn.InitiatorConnector.factory(initiator.ISCSI, utils.get_root_helper(),
                                                         use_multipath=True,
                                                         device_scan_attempts=CONF.libvirt.num_volume_scan_tries,
                                                         transport="default")
        device_info = conntest.connect_volume(data.get("data"))
        return device_info, 201
    else:
        return jsonify({"error": "Request must be JSON"}), 400

@app.route('/detachvolume', methods=['POST'])
def detach_volume(connection_info):
    if request.is_json:
        data = request.get_json()
        conntest = brick_conn.InitiatorConnector.factory(initiator.ISCSI, utils.get_root_helper(),
                                                         use_multipath=True,
                                                         device_scan_attempts=CONF.libvirt.num_volume_scan_tries,
                                                         transport="default")
        try:
            conntest.disconnect_volume(
                data.get("connection_info")['data'], None, force=False)
            return jsonify({"res":"sussfuly"}), 200
        except os_brick_exception.VolumeDeviceNotFound as exc:
            return

    else:
        return jsonify({"error": "Request must be JSON"}), 400

if __name__ == '__main__':
    # 运行Flask应用，默认地址是http://127.0.0.1:5000/

    node_id = get_ecsnode_id()
    if node_id == None:
        print("get ecsnode_id failed")
        exit(1)

    app.run(debug=True)


brick_conn.InitiatorConnector.factory(initiator.RBD, utils.get_root_helper(),
                                                         use_multipath=False,
                                                         device_scan_attempts=CONF.libvirt.num_volume_scan_tries,
                                                         transport="default")