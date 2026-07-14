#!/usr/bin/env python3
import json, os, pty, signal, socket, sys, time
SOCKET='/tmp/l4d2-supervisor.sock'; STATUS='/tmp/l4d2-supervisor.json'
def request(command):
    client=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM); client.connect(SOCKET); client.sendall((command+'\n').encode())
    while True:
        data=client.recv(65536)
        if not data: break
        sys.stdout.buffer.write(data); sys.stdout.buffer.flush()
def run():
    try: os.unlink(SOCKET)
    except FileNotFoundError: pass
    pid,fd=pty.fork()
    if pid==0:
        args=os.environ.get('SRCDS_ARGS','-game left4dead2 -console -port 27015 +map c2m1_highway').split()
        os.execv('./srcds_run',['./srcds_run',*args])
    server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM); server.bind(SOCKET); os.chmod(SOCKET,0o600); server.listen(4); started=time.time()
    with open(STATUS,'w') as f: json.dump({'pid':pid,'started_at':started,'last_exit_code':None},f)
    while True:
        conn,_=server.accept(); command=conn.recv(64).decode().strip()
        if command=='status': conn.sendall(json.dumps({'pid':pid,'uptime':int(time.time()-started),'running':True}).encode())
        elif command=='stop': os.write(fd,b'quit\n'); conn.sendall(b'{"accepted":true}')
        elif command=='attach':
            try:
                while True: conn.sendall(os.read(fd,4096))
            except (BrokenPipeError,OSError): pass
        conn.close()
if __name__=='__main__':
    command=sys.argv[1] if len(sys.argv)>1 else ''
    if command=='run': run()
    elif command in ('attach','stop','status'): request(command)
    else: sys.exit(64)
