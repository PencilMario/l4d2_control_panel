import {render,screen} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {describe,expect,it,vi} from 'vitest'
import {App, type Instance} from './App'
const instance:Instance={id:'1',name:'深夜战役',actual_state:'running',game_port:27015,start_map:'c2m1_highway',game_mode:'coop',max_players:8,players:4,cpu:31,memory:2.4}
describe('App',()=>{it('shows operational instance data',()=>{render(<App initialInstances={[instance]}/>);expect(screen.getByText('深夜战役')).toBeInTheDocument();expect(screen.getByText('4 / 8')).toBeInTheDocument();expect(screen.getByText('c2m1_highway')).toBeInTheDocument()});it('requires confirmation before stopping',async()=>{const onAction=vi.fn();render(<App initialInstances={[instance]} onAction={onAction}/>);await userEvent.click(screen.getByRole('button',{name:'停止 深夜战役'}));expect(onAction).not.toHaveBeenCalled();expect(screen.getByRole('dialog')).toBeInTheDocument();await userEvent.click(screen.getByRole('button',{name:'确认停止'}));expect(onAction).toHaveBeenCalledWith('1','stop')})})
