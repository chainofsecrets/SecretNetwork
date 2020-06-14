use snafu::ResultExt;

use cosmwasm::encoding::Binary;
use cosmwasm::errors::{contract_err, unauthorized, Result, SerializeErr};
use cosmwasm::serde::to_vec;
use cosmwasm::traits::{Api, Extern, Storage};
use cosmwasm::types::{log, CosmosMsg, Env, HumanAddr, Response};

use crate::msg::{HandleMsg, InitMsg, OwnerResponse, QueryMsg};
use crate::state::{config, config_read, State};

pub fn init<S: Storage, A: Api>(
    deps: &mut Extern<S, A>,
    env: Env,
    _msg: InitMsg,
) -> Result<Response> {
    Ok(Response::default())
}

pub fn handle<S: Storage, A: Api>(
    deps: &mut Extern<S, A>,
    env: Env,
    msg: HandleMsg,
) -> Result<Response> {
    match msg {
        HandleMsg::A {
            contract_addr,
            x,
            y,
        } => try_a(deps, env, contract_addr, x, y),
        HandleMsg::B { x, y } => try_b(deps, env, x, y),
    }
}

pub fn try_a<S: Storage, A: Api>(
    deps: &mut Extern<S, A>,
    env: Env,
    contract_addr: HumanAddr,
    x: u8,
    y: u8,
) -> Result<Response> {
    let res = Response {
        messages: vec![CosmosMsg::Contract {
            contract_addr,
            msg: Binary(
                format!("{{\"b\":{{\"x\":{} ,\"y\": {} }}}}", x, y)
                    .as_bytes()
                    .to_vec(),
            ),
            send: None,
        }],
        log: vec![log("action", "banana")],
        data: Some(Binary(vec![x, y])),
    };
    Ok(res)
}

pub fn try_b<S: Storage, A: Api>(
    deps: &mut Extern<S, A>,
    env: Env,
    x: u8,
    y: u8,
) -> Result<Response> {
    let res = Response {
        messages: vec![],
        log: vec![log("action", "papaya")],
        data: Some(Binary(vec![x + y])),
    };
    Ok(res)
}

pub fn query<S: Storage, A: Api>(deps: &Extern<S, A>, msg: QueryMsg) -> Result<Vec<u8>> {
    match msg {
        QueryMsg::Owner {} => query_owner(deps),
    }
}

fn query_owner<S: Storage, A: Api>(deps: &Extern<S, A>) -> Result<Vec<u8>> {
    let state = config_read(&deps.storage).load()?;

    let resp = OwnerResponse {
        owner: deps.api.human_address(&state.owner)?,
    };
    to_vec(&resp).context(SerializeErr {
        kind: "OwnerResponse",
    })
}
