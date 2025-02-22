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


def iscsi_common(data, action):
    iscs_conn = brick_conn.InitiatorConnector.factory(initiator.ISCSI, utils.get_root_helper(),
                                                      use_multipath=True,
                                                      device_scan_attempts=CONF.libvirt.num_volume_scan_tries,
                                                      transport="default")
    if action == 'attach':
        try:
            device_info = iscs_conn.connect_volume(data)
            return device_info, 200
        except:
            print("iscsi connent volume failed")
            return None, 500
    elif action == 'detach':
        try:
            iscs_conn.disconnect_volume(
                data, None, force=False)
            return jsonify({"res": "sussfuly"}), 200
        except os_brick_exception.VolumeDeviceNotFound as exc:
            return None, 500

    print("action not found")
    return None, 404


def fc_common(data, action):
    fc_conn = brick_conn.InitiatorConnector.factory(initiator.FIBRE_CHANNEL, utils.get_root_helper(),
                                                    use_multipath=True,
                                                    device_scan_attempts=CONF.libvirt.num_volume_scan_tries,
                                                    transport="default")
    if action == 'attach':
        try:
            device_info = fc_conn.connect_volume(data)
            return device_info, 200
        except:
            print("fc connent volume failed")
            return  None, 500
    elif action == 'detach':
        try:
            fc_conn.disconnect_volume(
                data, None, force=False)
            return jsonify({"res": "sussfuly"}), 200
        except os_brick_exception.VolumeDeviceNotFound as exc:
            return None, 500

    print("action not found")
    return None, 404


def rbd_common(data, action):
    if action == "attach":
        try:
            fetrue_command  = "/usr/bin/rbd info {0}| grep -i deep-flatten".format(data["name"])
            fetrue_result = subprocess.run(fetrue_command, shell=True, stderr=subprocess.STDOUT)
            if fetrue_result.returncode == 0:
                disable_futrue_commnad = "/usr/bin/rbd feature disable {0} object-map fast-diff deep-flatten".format(data["name"])
                subprocess.check_output(disable_futrue_commnad, shell=True, stderr=subprocess.STDOUT)
            rbd_attach_command = "/usr/bin/rbd map {0}".format(data["name"])
            device_info = subprocess.check_output(rbd_attach_command, shell=True, stderr=subprocess.STDOUT).decode('utf-8').strip()
            print(device_info)
            return {"path": device_info}, 200
        except subprocess.CalledProcessError as e:
            print("rbd map volume failed", e)
            return None, 500
    elif action == "detach":
        try:
            rbd_detach_command = "/usr/bin/rbd unmap {0}".format(data["name"])
            subprocess.check_output(rbd_detach_command, shell=True, stderr=subprocess.STDOUT)
            return {"res": "sussfuly"}, 200
        except subprocess.CalledProcessError as e:
            print("rbd map volume failed", e)
            return None, 500

    print("action not found")
    return None, 404





def get_ecsnode_id():
    ecsnode_json_command = "kubectl get ecsnode -n openstack -ojson"
    ecsnode_result = subprocess.check_output(ecsnode_json_command, shell=True, stderr=subprocess.STDOUT)

    hostname_command = "hostname | awk -F . '{print $1}'"
    hostname_result = subprocess.check_output(hostname_command, shell=True, stderr=subprocess.STDOUT).decode('utf-8').strip()



    res = json.loads(ecsnode_result)

    print(hostname_result)

    for item in res['items']:
        print(item['metadata']['name'])
        if item['data']['hostname'] == hostname_result:
            return item['metadata']['name']

    return None


@app.route('/ecsnode_id', methods=['POST'])
def get_ecsnode():
    return node_id

# "driver_volume_type"

@app.route('/connector', methods=['POST'])
def get_connector():
    if request.is_json:
        connector = brick_conn.get_connector_properties(root_helper=utils.get_root_helper(),
                                                        my_ip=CONF.my_block_storage_ip, multipath=False,
                                                        enforce_multipath=False, host=CONF.host)
        return connector, 200
    else:
        return jsonify({"error": "Request must be JSON"}), 400

@app.route('/attachvolume', methods=['POST'])
def attach_volume():
    print("123123 att")
    print(request)
    if request.is_json:
        print("123 att")
        data = request.get_json()
        print(data)
        driver = data["driver_volume_type"]
        print(driver)
        if driver:
            if driver == "iscsi":
                device_info, code = iscsi_common(data, "attach")
            elif driver ==  "fibre_channel":
                device_info, code = fc_common(data, "attach")
            elif driver == "rbd":
                device_info, code = rbd_common(data, "attach")
            else:
                device_info, code = None, 402

        print(device_info, code)
        if device_info is None or code == 404:
            print("no supported")
            return jsonify({"error": "driver not supported"}), 404

        return device_info, code
    else:
        print("123123123 json must attach")
        return jsonify({"error": "Request must be JSON"}), 401

@app.route('/detachvolume', methods=['POST'])
def detach_volume():
    if request.is_json:
        data = request.get_json()
        print(data)
        driver = data["driver_volume_type"]
        print(driver)
        if driver:
            if driver == "iscsi":
                device_info, code = iscsi_common(data, "detach")
            elif driver ==  "fibre_channel":
                device_info, code = fc_common(data, "detach")
            elif driver == "rbd":
                device_info, code = rbd_common(data, "detach")
            else:
                device_info, code = None, 404

        print(device_info, code)
        if  code != 200:
            print("detach failed")
            return jsonify({"error": "detach failed"}), code

        return device_info, code
    else:
        print("123123123 json must detach")
        return jsonify({"error": "Request must be JSON"}), 400







if __name__ == '__main__':
    # 运行Flask应用，默认地址是http://127.0.0.1:5000/

    BRI_ISCSI="iscsi"
    BRI_FIBER="fibre_channel"
    BRI_RBD="rbd"

    node_id = get_ecsnode_id()
    if node_id == None:
        print("get ecsnode_id failed")
        exit(1)


    app.run(debug=True, host="0.0.0.0")