import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';

import { PolicyComponentAction } from '../policy-component-action.enum';
import { PasswordAgePolicyComponent } from './password-age-policy.component';

const routes: Routes = [
    {
        path: '',
        component: PasswordAgePolicyComponent,
        data: {
            animation: 'DetailPage',
            action: PolicyComponentAction.MODIFY,
        },
    },
    {
        path: 'create',
        component: PasswordAgePolicyComponent,
        data: {
            animation: 'DetailPage',
            action: PolicyComponentAction.CREATE,
        },
    },
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule],
})
export class PasswordAgePolicyRoutingModule { }
