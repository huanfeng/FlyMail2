/**
 * 一个地址块该给哪些操作。
 *
 * 抽成纯函数是为了能钉住下面这条安全判据——它写在组件里就只能靠点开菜单去看。
 */
export interface AddressActions {
  /** 「总是显示此发件人的远程内容」 */
  canTrust: boolean
  /** 「屏蔽此发件人」 */
  canBlock: boolean
}

export function addressActions(role: 'from' | 'to', isSelf: boolean, email: string): AddressActions {
  // ⚠ 自己的地址绝不给屏蔽入口。
  //
  // 「已发送」里每一封的发件人都是自己，抄送里也常有自己的其它信箱。点一下就把
  // 自己拉黑，之后所有自发自收、抄送自己的邮件都进垃圾箱，而用户完全不知道发生
  // 了什么。后端也会拒，但这个菜单项压根不该出现在那里。
  //
  // 收件人不给这两项是另一个理由：它们管的是「这个人发来的邮件」怎么处理，
  // 挂在收件人上会让人以为是在管发给他的邮件。
  const actionable = role === 'from' && !isSelf && email.trim() !== ''
  return { canTrust: actionable, canBlock: actionable }
}
