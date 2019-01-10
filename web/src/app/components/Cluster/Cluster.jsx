/*
Copyright 2015 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import React from 'react';
import styled from 'styled-components';
import { Route, Switch, NavLink } from 'react-router-dom'
import AppBar from './../AppBar/AppBar';
import AppLogo from './../AppLogo';
import { connect } from './../nuclear';
import cfg from 'app/config';
import clusterGetters from 'app/flux/sites/getters';
import { Flex, Box, SideNav, SideNavItem } from 'shared/components';
import ClusterNodes from './../ClusterNodes';
import ClusterSessions from './../ClusterSessions';
import ClusterSelector from './ClusterSelector';
import { changeCluster } from 'app/flux/sites/actions';


export class Cluster extends React.Component {

  onChangeCluster = option => {
    this.props.onChangeCluster(option.value)
  }

  render() {
    const { clusterId, clusters } = this.props;
    const clusterOptions = clusters.map(c => ({
      value: c.name,
      label: c.name
    }));

    return (
      <Flex height="100%">
        <SideNav>
          <AppLogo />
          <SideNavItem as={props => (
            <NavLink className={props.className}
              exact
              to={cfg.getClusterUrl(clusterId)}
            >
              Nodes
            </NavLink>
            )}
          />
          <SideNavItem as={props => (
            <NavLink className={props.className}
              to={cfg.getClusterSessionsUrl(clusterId)}>
              Audit Log
            </NavLink>
            )}
          />
        </SideNav>
        <Flex flexDirection="column" width="100%" px={5}>
          <div>
            <ClusterSelector
              value={clusterId}
              onChange={this.onChangeCluster}
              options={clusterOptions}
            />
            <AppBar />
          </div>
          <Content>
            <Switch>
              <Route exact path={cfg.routes.cluster} >
                <ClusterNodes clusterId={clusterId} />
              </Route>
              <Route exact path={cfg.routes.clusterSessions} >
                <ClusterSessions clusterId={clusterId} />
              </Route>
            </Switch>
          </Content>
        </Flex>
      </Flex>
    );
  }
}

function mapStoreToProps() {
  return {
    clusters: clusterGetters.sites
  }
}

function mapStateToProps() {
  return {
    onChangeCluster: changeCluster,
  }
}

const Content = styled(Box)`
  overflow: auto;
  width: 100%;
  height: 100%;
`

export default connect(mapStoreToProps, mapStateToProps)(Cluster);


