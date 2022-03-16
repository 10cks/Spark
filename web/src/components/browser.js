import React, {useEffect, useRef, useState} from 'react';
import {message, Modal, Popconfirm} from "antd";
import ProTable from '@ant-design/pro-table';
import {formatSize, post, request, waitTime} from "../utils/utils";
import './browser.css';
import dayjs from "dayjs";

function FileBrowser(props) {
    const [path, setPath] = useState(`/`);
    const [loading, setLoading] = useState(false);
    const columns = [
        {
            key: 'Name',
            title: 'Name',
            dataIndex: 'name',
            ellipsis: true,
            width: 180
        },
        {
            key: 'Size',
            title: 'Size',
            dataIndex: 'size',
            ellipsis: true,
            width: 60,
            renderText: (size, file) => file.type === 0 ? formatSize(size) : '-'
        },
        {
            key: 'Time',
            title: 'Time Modified',
            dataIndex: 'time',
            ellipsis: true,
            width: 100,
            renderText: (ts, file) => file.type === 0 ? dayjs.unix(ts).format('YYYY/MM/DD HH:mm') : '-'
        },
        {
            key: 'Option',
            width: 120,
            title: '操作',
            dataIndex: 'name',
            valueType: 'option',
            ellipsis: true,
            render: (_, file) => renderOperation(file)
        },
    ];
    const options = {
        show: true,
        density: false,
        setting: false,
    };
    const tableRef = useRef();
    useEffect(() => {
        setPath(`/`);
        if (props.visible) {
            setLoading(false);
        }
    }, [props.device, props.visible]);

    function renderOperation(file) {
        let remove = (
            <Popconfirm
                key='remove'
                title={'确定要删除该' + (file.type === 0 ? '文件' : '目录') + '吗？'}
                onConfirm={removeFile.bind(null, file.name)}
            >
                <a>删除</a>
            </Popconfirm>
        );
        switch (file.type) {
            case 0:
                return [
                    <a
                        key='download'
                        onClick={downloadFile.bind(null, file.name)}
                    >下载</a>,
                    remove,
                ];
            case 1:
                return [remove];
            case 2:
                return [];
        }
        return [];
    }

    function onRowClick(file) {
        let separator = props.isWindows ? '\\' : '/';
        if (file.name === '..') {
            listFiles(getLastPath());
            return;
        }
        if (file.type !== 0) {
            if (props.isWindows) {
                if (path === '/' || path === '\\' || path.length === 0) {
                    listFiles(file.name + separator);
                    return
                }
            }
            listFiles(path + file.name + separator);
        }
    }

    function listFiles(newPath) {
        setPath(newPath);
        tableRef.current.reload();
    }

    function getLastPath() {
        let separator = props.isWindows ? '\\' : '/';
        // remove the last separator
        // or there'll be an empty element after split
        let tempPath = path.substring(0, path.length - 1);
        let pathArr = tempPath.split(separator);
        // remove current folder
        pathArr.pop();
        // back to root folder
        if (pathArr.length === 0) {
            return `/`;
        }
        return pathArr.join(separator) + separator;
    }

    function downloadFile(file) {
        post(location.origin + location.pathname + 'api/device/file/get', {
            file: path + file,
            device: props.device
        });
    }

    function removeFile(file) {
        request(`/api/device/file/remove`, {path: path+file, device: props.device}).then(res => {
            let data = res.data;
            if (data.code === 0) {
                message.success('文件或目录已被删除');
                tableRef.current.reload();
            }
        });
    }

    async function getData(form) {
        await waitTime(300);
        let res = await request('/api/device/file/list', {path: path, device: props.device});
        setLoading(false);
        let data = res.data;
        if (data.code === 0) {
            let addParentShortcut = false;
            data.data.files = data.data.files.sort((first, second) => (second.type - first.type));
            if (path.length > 0 && path !== '/' && path !== '\\') {
                addParentShortcut = true;
                data.data.files.unshift({
                    name: '..',
                    size: '0',
                    type: 3,
                    modTime: 0
                });
            }
            return ({
                data: data.data.files,
                success: true,
                total: data.data.files.length - (addParentShortcut?1:0)
            });
        }
        setPath(getLastPath());
        return ({data: [], success: false, total: 0});
    }

    return (
        <Modal
            destroyOnClose={true}
            title='File Explorer'
            footer={null}
            height={500}
            width={800}
            bodyStyle={{
                padding: 0
            }}
            {...props}
        >
            <ProTable
                rowKey='name'
                onRow={file => ({
                    onDoubleClick: onRowClick.bind(null, file),
                })}
                tableStyle={{
                    minHeight: '350px',
                    maxHeight: '350px'
                }}
                toolbar={{
                    actions: []
                }}
                scroll={{scrollToFirstRowOnChange: true, y: 300}}
                search={false}
                size='small'
                loading={loading}
                rowClassName='file-row'
                onLoadingChange={setLoading}
                options={options}
                columns={columns}
                request={getData}
                pagination={false}
                actionRef={tableRef}
            >

            </ProTable>
        </Modal>
    )
}

export default FileBrowser;